// Package capture is a gin middleware that records every upstream request
// and response into the prism_traces table. It is deliberately scoped to
// the relay router group so it doesn't observe admin/auth traffic.
package capture

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"

	onehubmodel "one-api/model"
	"prism/backend/prism/trace"
)

// Cap per trace body to avoid OOM under long streaming responses or giant
// payloads. 2MB each direction is plenty for debugging; anything larger
// gets truncated with a marker so the user can still see "something was
// there" without crashing the desktop app.
const maxBodyBytes = 2 * 1024 * 1024

const truncatedMarker = "\n...[truncated by prism capture]..."

// shouldCapture returns true for paths that are actual upstream relay
// traffic. We explicitly skip admin/dashboard/sse so the capture table
// stays focused on "what the user's AI clients sent and received".
func shouldCapture(path string) bool {
	switch {
	case strings.HasPrefix(path, "/api/"),
		strings.HasPrefix(path, "/dashboard/"),
		path == "/status",
		path == "/ping":
		return false
	}
	// Every prefix below is a relay endpoint configured in
	// core/router/relay-router.go. We allow-list rather than block so new
	// admin routes never accidentally land in the trace log.
	switch {
	case strings.HasPrefix(path, "/v1/"),
		strings.HasPrefix(path, "/claude/"),
		strings.HasPrefix(path, "/gemini/"),
		strings.HasPrefix(path, "/mj/"),
		strings.HasPrefix(path, "/suno/"),
		strings.HasPrefix(path, "/kling/"),
		strings.HasPrefix(path, "/recraftAI/"):
		return true
	}
	// "/:mode/mj" style paths — accept anything that has /mj/ anywhere after the prefix.
	if strings.Contains(path, "/mj/") {
		return true
	}
	return false
}


// tee wraps gin.ResponseWriter so every Write is mirrored into buf.
type tee struct {
	gin.ResponseWriter
	buf      *bytes.Buffer
	total    int64
	overflow bool
}

func (t *tee) Write(p []byte) (int, error) {
	n, err := t.ResponseWriter.Write(p)
	t.total += int64(n)
	if !t.overflow {
		remaining := maxBodyBytes - t.buf.Len()
		if remaining <= 0 {
			t.overflow = true
		} else if n > remaining {
			t.buf.Write(p[:remaining])
			t.buf.WriteString(truncatedMarker)
			t.overflow = true
		} else {
			t.buf.Write(p[:n])
		}
	}
	return n, err
}

func (t *tee) WriteString(s string) (int, error) {
	return t.Write([]byte(s))
}

// Middleware returns a gin.HandlerFunc that saves traces. Safe even when
// the prism_traces table hasn't been migrated — Save will simply return an
// error which we swallow so the proxy never breaks.
func Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !shouldCapture(c.Request.URL.Path) {
			c.Next()
			return
		}
		start := time.Now()

		reqBody := sniffRequestBody(c)

		w := &tee{ResponseWriter: c.Writer, buf: &bytes.Buffer{}}
		c.Writer = w

		c.Next()

		contentType := c.Writer.Header().Get("Content-Type")
		isStream := strings.Contains(strings.ToLower(contentType), "text/event-stream")

		channelID := c.GetInt("channel_id")
		channelName := ""
		channelType := c.GetInt("channel_type")
		if channelID > 0 {
			if ch, err := onehubmodel.GetChannelById(channelID); err == nil && ch != nil {
				channelName = ch.Name
			}
		}

		model := c.GetString("original_model")
		if model == "" {
			model = gjson.GetBytes(reqBody, "model").String()
		}

		entry := &trace.Entry{
			CreatedAt:     start.Unix(),
			Method:        c.Request.Method,
			Path:          c.Request.URL.Path,
			Status:        c.Writer.Status(),
			DurationMs:    time.Since(start).Milliseconds(),
			IsStream:      isStream,
			ChannelID:     channelID,
			ChannelType:   channelType,
			ChannelName:   channelName,
			TokenName:     c.GetString("token_name"),
			Model:         model,
			ClientIP:      c.ClientIP(),
			ContentType:   contentType,
			RequestBytes:  int64(len(reqBody)),
			ResponseBytes: w.total,
			RequestBody:   safeString(reqBody),
			ResponseBody:  w.buf.String(),
		}

		if entry.Status >= 400 {
			entry.ErrorMessage = extractError(w.buf.Bytes(), contentType)
		}
		// one-hub wraps network failures as "请求上游地址失败" for the client but
		// attaches the raw error (dial/timeout/TLS) via c.Error. Prefer that
		// when present so the Traces UI surfaces the actionable cause.
		if ginErr := c.Errors.Last(); ginErr != nil {
			raw := ginErr.Err.Error()
			if raw != "" && raw != entry.ErrorMessage {
				if entry.ErrorMessage != "" {
					entry.ErrorMessage = entry.ErrorMessage + " | " + raw
				} else {
					entry.ErrorMessage = raw
				}
			}
		}

		if err := trace.Save(context.Background(), entry); err != nil {
			// Never let a logging failure escape; relay already happened.
			gin.DefaultErrorWriter.Write([]byte("prism trace save failed: " + err.Error() + "\n"))
		}
	}
}

// sniffRequestBody clones and restores the request body so downstream
// handlers (one-hub's UnmarshalBodyReusable) see an untouched reader.
func sniffRequestBody(c *gin.Context) []byte {
	if c.Request.Body == nil || c.Request.ContentLength == 0 {
		return nil
	}
	// Cap what we keep; still drain the full body into the request so
	// one-hub's validator reads exactly what the client sent.
	lr := io.LimitReader(c.Request.Body, maxBodyBytes+1)
	stored, err := io.ReadAll(lr)
	if err != nil {
		return nil
	}
	// If we hit the cap, drain the rest without keeping it.
	var rest []byte
	if len(stored) > maxBodyBytes {
		truncated := make([]byte, maxBodyBytes, maxBodyBytes+len(truncatedMarker))
		copy(truncated, stored[:maxBodyBytes])
		remainder, _ := io.ReadAll(c.Request.Body)
		rest = remainder
		truncated = append(truncated, []byte(truncatedMarker)...)
		// Restore a reader that still delivers the full original bytes.
		full := append(stored, rest...)
		c.Request.Body = io.NopCloser(bytes.NewReader(full))
		return truncated
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(stored))
	return stored
}

// safeString strips sentinel NULs from a body so sqlite text columns stay
// valid even if a client managed to send binary.
func safeString(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	if bytes.IndexByte(b, 0) < 0 {
		return string(b)
	}
	return string(bytes.ReplaceAll(b, []byte{0}, []byte(`\x00`)))
}

// extractError plucks a human-readable message out of common error shapes
// (OpenAI's {"error":{"message":...}}, plain text, or SSE).
func extractError(body []byte, contentType string) string {
	if len(body) == 0 {
		// http.StatusText(0) returns "" which is what the caller used to
		// get; be explicit so an empty error body still shows something
		// in the Logs UI instead of a blank ErrorMessage column.
		return "(empty response body)"
	}
	if strings.Contains(contentType, "json") {
		if msg := gjson.GetBytes(body, "error.message").String(); msg != "" {
			return msg
		}
		if msg := gjson.GetBytes(body, "message").String(); msg != "" {
			return msg
		}
		var anyJSON any
		if json.Unmarshal(body, &anyJSON) == nil {
			if b, err := json.Marshal(anyJSON); err == nil && len(b) < 512 {
				return string(b)
			}
		}
	}
	if len(body) > 512 {
		return string(body[:512]) + truncatedMarker
	}
	return string(body)
}
