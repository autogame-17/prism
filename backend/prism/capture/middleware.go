// Package capture is a gin middleware that records every upstream request
// and response into the prism_traces table. It is deliberately scoped to
// the relay router group so it doesn't observe admin/auth traffic.
//
// Streaming responses get incremental persistence: the row is inserted as
// soon as the first byte is written, then UPDATEd on a throttled cadence
// (every flushInterval or when flushBytes new bytes have accumulated), and
// Finalized once the upstream handler returns. This way a stream that gets
// killed mid-flight (cloudflared restart, client disconnect, panic, kill -9)
// still leaves enough trail in prism_traces for the user to see WHAT was
// in-flight and HOW FAR it got — instead of the previous behaviour where
// the whole row was lost or recorded as `response_body=""`.
package capture

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"

	onehubmodel "one-api/model"
	"prism/backend/prism/identity"
	"prism/backend/prism/trace"
)

// Cap per trace body to avoid OOM under long streaming responses or giant
// payloads. 2MB each direction is plenty for debugging; anything larger
// gets truncated with a marker so the user can still see "something was
// there" without crashing the desktop app.
const maxBodyBytes = 2 * 1024 * 1024

const truncatedMarker = "\n...[truncated by prism capture]..."

// Streaming flush throttle. We MUST NOT do a sqlite UPDATE on every Write
// — SSE chunks for chat completions arrive at 30-100 Hz with bodies of
// tens of bytes, and we'd ruin both write amplification and proxy
// latency. A window of either 64 KB or 250ms keeps loss bounded ("at
// most 250ms of in-flight content can be missing if the process dies")
// while keeping the steady-state DB write rate at a few Hz.
const (
	flushInterval = 250 * time.Millisecond
	flushBytes    = 64 * 1024
)

// requestIDHeader is what we inject on every captured request and read
// back when we want to correlate Prism rows to upstream gateway logs.
// HTTP header names are case-insensitive on the wire; we use the
// canonical Title-Case form here so anything that prints headers
// verbatim shows a recognisable shape.
const requestIDHeader = "X-Request-Id"

// newRequestID returns `prism-<deviceId>-<unixMilli>`. The trailing
// monotonic counter is added when two requests would otherwise land in
// the same millisecond on the same device, so the ID stays globally
// unique even under burst load. Falls back to a per-process token if
// the device id has not yet been initialised (early boot / tests).
func newRequestID(t time.Time) string {
	dev := identity.Get()
	if dev == "" {
		dev = "nodevice"
	}
	ms := t.UnixMilli()
	seq := nextRequestIDSeq(dev, ms)
	if seq == 0 {
		return fmt.Sprintf("prism-%s-%d", dev, ms)
	}
	return fmt.Sprintf("prism-%s-%d-%d", dev, ms, seq)
}

// nextRequestIDSeq returns 0 the first time it sees a (device, ms) pair
// and N>=1 on subsequent collisions within the same millisecond. The
// state is intentionally tiny — only the latest (device, ms) is
// remembered — because real collisions are rare and we don't want this
// to grow without bound.
var (
	ridMu      sync.Mutex
	ridLastKey string
	ridLastSeq int
)

func nextRequestIDSeq(device string, ms int64) int {
	key := fmt.Sprintf("%s-%d", device, ms)
	ridMu.Lock()
	defer ridMu.Unlock()
	if key == ridLastKey {
		ridLastSeq++
		return ridLastSeq
	}
	ridLastKey = key
	ridLastSeq = 0
	return 0
}

// flushSignal is one snapshot the flusher goroutine consumes. Carrying
// the whole body each time (rather than a delta) keeps the consumer
// dead-simple: it just writes the latest snapshot whenever it decides
// to flush. body is already a defensive copy when produced by tee.
type flushSignal struct {
	body   []byte
	total  int64
	chunks int
	first  int64
	last   int64
}

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


// tee wraps gin.ResponseWriter so every Write is mirrored into buf. When
// the upstream response sets Content-Type: text/event-stream, tee flips
// into streaming mode on the first Write and invokes onChunk after every
// subsequent Write. onChunk runs OUTSIDE tee.mu (we drop the lock after
// snapshotting) so a slow DB flush can never stall the proxy hot path.
type tee struct {
	gin.ResponseWriter
	mu       sync.Mutex
	buf      *bytes.Buffer
	total    int64
	overflow bool

	// streaming bookkeeping. headerInspected guards the one-shot
	// Content-Type check we do on the first Write.
	streaming       bool
	headerInspected bool
	chunkCount      int
	firstChunkAt    int64
	lastChunkAt     int64

	onChunk func(snapshot []byte, total int64, chunkCount int, firstAt, lastAt int64)
}

func (t *tee) Write(p []byte) (int, error) {
	n, err := t.ResponseWriter.Write(p)
	t.mu.Lock()
	if !t.headerInspected {
		t.headerInspected = true
		ct := strings.ToLower(t.ResponseWriter.Header().Get("Content-Type"))
		if strings.Contains(ct, "text/event-stream") {
			t.streaming = true
		}
	}
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
	streamed := t.streaming && n > 0
	if streamed {
		now := time.Now().UnixMilli()
		t.chunkCount++
		if t.firstChunkAt == 0 {
			t.firstChunkAt = now
		}
		t.lastChunkAt = now
	}
	cb := t.onChunk
	var (
		snap   []byte
		cnt    int
		first  int64
		last   int64
		totalN int64
	)
	if cb != nil && streamed {
		// Copy the buffer because the middleware hands it to a goroutine
		// that survives this Write; sharing the underlying array would
		// race subsequent Writes that grow buf.
		snap = append([]byte(nil), t.buf.Bytes()...)
		cnt = t.chunkCount
		first = t.firstChunkAt
		last = t.lastChunkAt
		totalN = t.total
	}
	t.mu.Unlock()
	if cb != nil && streamed {
		cb(snap, totalN, cnt, first, last)
	}
	return n, err
}

func (t *tee) WriteString(s string) (int, error) {
	return t.Write([]byte(s))
}

// snapshot returns a defensive copy of the captured state. Used by
// Finalize so the read cannot race a last-millisecond Write.
func (t *tee) snapshot() (body []byte, total int64, streaming bool, chunks int, first, last int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	body = append([]byte(nil), t.buf.Bytes()...)
	return body, t.total, t.streaming, t.chunkCount, t.firstChunkAt, t.lastChunkAt
}

// Middleware returns a gin.HandlerFunc that saves traces. Streaming
// responses are written incrementally so the row survives a mid-flight
// process death; non-streaming responses still use a single terminal
// Save like before. Safe even when the prism_traces table hasn't been
// migrated — every DB call is best-effort and a failure is logged but
// never escapes to the proxy hot path.
func Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !shouldCapture(c.Request.URL.Path) {
			c.Next()
			return
		}
		start := time.Now()

		rid := newRequestID(start)
		// Inject the request id BEFORE c.Next so one-hub forwards it to
		// the upstream provider verbatim, AND set it on the response so
		// the client (Cursor / IDE / curl) sees the same identifier it
		// can use to follow up with us.
		c.Request.Header.Set(requestIDHeader, rid)
		c.Writer.Header().Set(requestIDHeader, rid)
		c.Set("prism_request_id", rid)

		reqBody := sniffRequestBody(c)

		w := &tee{ResponseWriter: c.Writer, buf: &bytes.Buffer{}}
		c.Writer = w

		deviceID := identity.Get()

		// Streaming state. rowID stays 0 until the first SSE chunk
		// fires onChunk; at that point we synchronously insert a
		// placeholder row and spin up a flusher goroutine that drains
		// throttled snapshots into AppendChunk. doneSending guards a
		// late Write (e.g. gin.Recovery writing a 500 after we've torn
		// down the flusher) from racing with a closed stopCh.
		var (
			openOnce    sync.Once
			rowID       int64
			openErr     error
			flushCh     = make(chan flushSignal, 1)
			stopCh      = make(chan struct{})
			flushWG     sync.WaitGroup
			doneSending atomic.Bool
		)

		w.onChunk = func(snap []byte, total int64, chunks int, first, last int64) {
			if doneSending.Load() {
				return
			}
			openOnce.Do(func() {
				// We resolve as much routing context as is known by
				// the time the first byte hits the wire. Channel info
				// might still be empty here if one-hub set it later;
				// Finalize will fill in the gaps.
				modelAtOpen := c.GetString("original_model")
				if modelAtOpen == "" {
					modelAtOpen = gjson.GetBytes(reqBody, "model").String()
				}
				ent := &trace.Entry{
					CreatedAt:    start.Unix(),
					RequestID:    rid,
					DeviceID:     deviceID,
					Method:       c.Request.Method,
					Path:         c.Request.URL.Path,
					IsStream:     true,
					ChannelID:    c.GetInt("channel_id"),
					ChannelType:  c.GetInt("channel_type"),
					TokenName:    c.GetString("token_name"),
					Model:        modelAtOpen,
					ClientIP:     c.ClientIP(),
					ContentType:  w.Header().Get("Content-Type"),
					RequestBytes: int64(len(reqBody)),
					RequestBody:  safeString(reqBody),
					Finished:     false,
				}
				id, err := trace.OpenStream(context.Background(), ent)
				if err != nil {
					openErr = err
					return
				}
				rowID = id
				startStreamFlusher(rowID, flushCh, stopCh, &flushWG)
			})

			if openErr != nil || rowID == 0 {
				// OpenStream failed; degrade to terminal Save at the
				// end. We still drop the snapshot rather than queue
				// unbounded, so memory stays bounded.
				return
			}
			// Non-blocking send: if the flusher hasn't consumed the
			// previous snapshot, replace it. The flusher always writes
			// the newest snapshot, so dropping the older one loses
			// nothing.
			select {
			case flushCh <- flushSignal{body: snap, total: total, chunks: chunks, first: first, last: last}:
			default:
				select {
				case <-flushCh:
				default:
				}
				select {
				case flushCh <- flushSignal{body: snap, total: total, chunks: chunks, first: first, last: last}:
				default:
				}
			}
		}

		// Run the proxy. If the upstream handler panics, gin.Recovery
		// converts it into a 500 BEFORE we get here; we still finalize
		// below with the bytes captured so far.
		c.Next()

		// Mark sending done BEFORE closing stopCh so a late Write (gin
		// recovery writing a 500 after we tore down the flusher) is a
		// no-op rather than a send on a stopped pipeline.
		doneSending.Store(true)
		// Stop the flusher (if it was started). The flusher applies one
		// last pending snapshot before returning, so we don't lose the
		// most recent chunks.
		close(stopCh)
		flushWG.Wait()

		body, totalBytes, isStreamSeen, chunks, first, last := w.snapshot()
		contentType := c.Writer.Header().Get("Content-Type")
		// We trust either signal: the tee's runtime detection (we saw
		// SSE chunks) OR the final Content-Type header. They normally
		// agree, but the header may not be set if the upstream returned
		// an error before writing anything.
		isStream := isStreamSeen || strings.Contains(strings.ToLower(contentType), "text/event-stream")

		channelID := c.GetInt("channel_id")
		channelName := ""
		channelType := c.GetInt("channel_type")
		if channelID > 0 {
			if ch, err := onehubmodel.GetChannelById(channelID); err == nil && ch != nil {
				channelName = ch.Name
			}
		}

		modelName := c.GetString("original_model")
		if modelName == "" {
			modelName = gjson.GetBytes(reqBody, "model").String()
		}

		status := c.Writer.Status()
		errMsg := ""
		if status >= 400 {
			errMsg = extractError(body, contentType)
		}
		if ginErr := c.Errors.Last(); ginErr != nil {
			raw := ginErr.Err.Error()
			if raw != "" && raw != errMsg {
				if errMsg != "" {
					errMsg = errMsg + " | " + raw
				} else {
					errMsg = raw
				}
			}
		}

		// Streaming "finished cleanly" heuristic: status<400 AND the
		// SSE terminator was observed in the body. Anything else means
		// the stream was cut short — that's exactly the signal the
		// laundry pipeline (and the user investigating "no response")
		// needs to attribute the loss correctly.
		finishReason := extractFinishReason(body)
		streamClean := status < 400 && bytes.Contains(body, []byte("data: [DONE]"))

		// Branch on whether we ever opened a streaming placeholder.
		if rowID != 0 {
			ferr := trace.Finalize(context.Background(), rowID, trace.FinalizeStreamUpdate{
				Status:        status,
				DurationMs:    time.Since(start).Milliseconds(),
				ResponseBody:  safeString(body),
				ResponseBytes: totalBytes,
				ContentType:   contentType,
				IsStream:      isStream,
				ChannelID:     channelID,
				ChannelType:   channelType,
				ChannelName:   channelName,
				Model:         modelName,
				ChunkCount:    chunks,
				FirstChunkAt:  first,
				LastChunkAt:   last,
				Finished:      streamClean,
				FinishReason:  finishReason,
				ErrorMessage:  errMsg,
			})
			if ferr != nil {
				gin.DefaultErrorWriter.Write([]byte("prism trace finalize failed: " + ferr.Error() + "\n"))
			}
			return
		}

		// Non-streaming (or streaming-but-no-bytes) path: single insert.
		entry := &trace.Entry{
			CreatedAt:     start.Unix(),
			RequestID:     rid,
			DeviceID:      deviceID,
			Method:        c.Request.Method,
			Path:          c.Request.URL.Path,
			Status:        status,
			DurationMs:    time.Since(start).Milliseconds(),
			IsStream:      isStream,
			Finished:      status < 400, // non-stream success = finished
			FinishReason:  finishReason,
			ChunkCount:    chunks,
			FirstChunkAt:  first,
			LastChunkAt:   last,
			ChannelID:     channelID,
			ChannelType:   channelType,
			ChannelName:   channelName,
			TokenName:     c.GetString("token_name"),
			Model:         modelName,
			ClientIP:      c.ClientIP(),
			ContentType:   contentType,
			ErrorMessage:  errMsg,
			RequestBytes:  int64(len(reqBody)),
			ResponseBytes: totalBytes,
			RequestBody:   safeString(reqBody),
			ResponseBody:  safeString(body),
		}
		if err := trace.Save(context.Background(), entry); err != nil {
			gin.DefaultErrorWriter.Write([]byte("prism trace save failed: " + err.Error() + "\n"))
		}
	}
}

// startStreamFlusher drains snapshots from ch and persists them with a
// throttle of either flushInterval (time-based) or flushBytes (size-based,
// applied at receive time). Exits when stopCh is closed, applying one
// final pending snapshot so the last few chunks before shutdown survive.
func startStreamFlusher(rowID int64, ch <-chan flushSignal, stopCh <-chan struct{}, wg *sync.WaitGroup) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(flushInterval)
		defer ticker.Stop()

		var (
			pending     *flushSignal
			lastFlushed int64
		)
		apply := func() {
			if pending == nil {
				return
			}
			err := trace.AppendChunk(context.Background(), rowID, trace.StreamChunkUpdate{
				ResponseBody:  safeString(pending.body),
				ResponseBytes: pending.total,
				ChunkCount:    pending.chunks,
				FirstChunkAt:  pending.first,
				LastChunkAt:   pending.last,
			})
			if err == nil {
				lastFlushed = pending.total
			}
			pending = nil
		}
		for {
			select {
			case sig := <-ch:
				s := sig
				pending = &s
				// Size-based fast path: if we've accumulated more than
				// flushBytes since the last successful flush, persist
				// immediately so a sudden burst doesn't wait the full
				// 250ms.
				if pending.total-lastFlushed >= flushBytes {
					apply()
				}
			case <-ticker.C:
				apply()
			case <-stopCh:
				// Drain anything still queued, then write one final
				// snapshot before exiting.
				select {
				case sig := <-ch:
					s := sig
					pending = &s
				default:
				}
				apply()
				return
			}
		}
	}()
}

// extractFinishReason reads the last `finish_reason` field present in an
// SSE body. Returning the most recent non-empty value matches OpenAI's
// own semantics where the terminating chunk carries the reason.
func extractFinishReason(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	const marker = `"finish_reason":`
	last := ""
	idx := 0
	for {
		j := bytes.Index(body[idx:], []byte(marker))
		if j < 0 {
			break
		}
		// Move past the marker and read the JSON value (string or null).
		k := idx + j + len(marker)
		// Skip whitespace.
		for k < len(body) && (body[k] == ' ' || body[k] == '\t') {
			k++
		}
		if k >= len(body) {
			break
		}
		if body[k] == 'n' {
			// `null` — skip.
			idx = k + 4
			continue
		}
		if body[k] != '"' {
			idx = k
			continue
		}
		// Read until closing quote.
		end := k + 1
		for end < len(body) && body[end] != '"' {
			end++
		}
		if end >= len(body) {
			break
		}
		last = string(body[k+1 : end])
		idx = end + 1
	}
	return last
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
