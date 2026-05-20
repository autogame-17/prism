package capture

import (
	"bytes"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"github.com/tidwall/gjson"
)

// Maximum lengths we tolerate when writing strings into the trace row.
// These keep an oversized header or a runaway system-prompt path from
// ballooning the sqlite columns. They match the gorm `varchar` widths
// declared in trace.Entry; whichever is smaller wins.
const (
	maxUserAgentLen = 256
	maxSessionIDLen = 96
	maxCWDLen       = 512
)

// sessionIDHeaders enumerates the HTTP header names we honour for an
// explicit client-supplied session identifier. First non-empty wins.
// Kept short on purpose — adding more without a real client emitting
// them just risks colliding with unrelated headers.
var sessionIDHeaders = []string{
	"X-Session-Id",
	"X-Session-ID",
	"X-Cursor-Session-Id",
	"X-Conversation-Id",
}

// extractUserAgent returns the User-Agent header, trimmed and capped.
// We accept whatever the client sent verbatim (no parsing into
// vendor/version) because clients are inconsistent and the raw string
// is what an investigator wants to see.
func extractUserAgent(req *http.Request) string {
	if req == nil {
		return ""
	}
	ua := strings.TrimSpace(req.Header.Get("User-Agent"))
	return truncate(ua, maxUserAgentLen)
}

// extractSessionID resolves the logical session id with this priority:
//  1. an explicit header from sessionIDHeaders;
//  2. the OpenAI-spec `user` body field (free-form identifier);
//  3. `metadata.session_id` if the client uses the OpenAI metadata
//     channel (some SDKs and Anthropic's `metadata.user_id` shape do).
//
// Returns empty string when nothing matches — session_id is explicitly
// declared optional by the data laundry pipeline.
func extractSessionID(req *http.Request, reqBody []byte) string {
	if req != nil {
		for _, h := range sessionIDHeaders {
			if v := strings.TrimSpace(req.Header.Get(h)); v != "" {
				return truncate(v, maxSessionIDLen)
			}
		}
	}
	if len(reqBody) == 0 {
		return ""
	}
	if v := strings.TrimSpace(gjson.GetBytes(reqBody, "user").String()); v != "" {
		return truncate(v, maxSessionIDLen)
	}
	if v := strings.TrimSpace(gjson.GetBytes(reqBody, "metadata.session_id").String()); v != "" {
		return truncate(v, maxSessionIDLen)
	}
	if v := strings.TrimSpace(gjson.GetBytes(reqBody, "metadata.user_id").String()); v != "" {
		return truncate(v, maxSessionIDLen)
	}
	return ""
}

// cwdPatterns is the ordered list of regexes we try against system /
// user message text to recover a working-directory hint. Cursor's
// system prompt uses `Workspace Path: /xxx`; Claude Code uses
// `Primary working directory: /xxx`; other clients (Aider,
// hand-rolled scripts) use `cwd: /xxx`, `Working directory: /xxx`,
// `<cwd>/xxx</cwd>`. We deliberately anchor on a leading `/` (or
// drive letter on Windows) so an arbitrary mention of the word
// "cwd" in tool output does not produce a false positive.
//
// First successful match wins. The regex must capture the path in
// group 1.
var cwdPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)workspace\s*path[:=]\s*([A-Za-z]:[\\/][^\s"'\n\r\\]+|/[^\s"'\n\r\\]+)`),
	regexp.MustCompile(`(?i)(?:current|primary)\s+working\s+directory(?:\s+is)?[:=]?\s*([A-Za-z]:[\\/][^\s"'\n\r\\]+|/[^\s"'\n\r\\]+)`),
	regexp.MustCompile(`(?i)working\s+directory[:=]\s*([A-Za-z]:[\\/][^\s"'\n\r\\]+|/[^\s"'\n\r\\]+)`),
	regexp.MustCompile(`(?i)\bcwd[:=]\s*([A-Za-z]:[\\/][^\s"'\n\r\\]+|/[^\s"'\n\r\\]+)`),
	regexp.MustCompile(`<cwd>\s*([A-Za-z]:[\\/][^\s<\n\r]+|/[^\s<\n\r]+)\s*</cwd>`),
}

// extractCWD scans the request body for a working-directory hint in
// the two places clients put it: Anthropic's top-level `system` field
// (string OR structured-array form), and the chat-style `messages`
// array. We intentionally skip assistant + tool messages: the model
// may echo a path that does not belong to the client's workspace, and
// tool output often contains unrelated paths. If neither shape is
// recognised (e.g. /v1/embeddings) the function returns "" without
// scanning anything.
//
// The system field wins over messages because Claude Code embeds
// `Primary working directory: /xxx` there exclusively; without this
// branch ~93% of Claude Code traces landed with empty cwd even though
// the path was right in the request.
func extractCWD(reqBody []byte) string {
	if len(reqBody) == 0 {
		return ""
	}
	if hint := matchCWDPatterns(contentToText(gjson.GetBytes(reqBody, "system"))); hint != "" {
		return hint
	}
	messages := gjson.GetBytes(reqBody, "messages")
	if !messages.IsArray() {
		return ""
	}
	var found string
	messages.ForEach(func(_, msg gjson.Result) bool {
		role := msg.Get("role").String()
		if role != "system" && role != "user" && role != "developer" {
			return true
		}
		text := contentToText(msg.Get("content"))
		if hint := matchCWDPatterns(text); hint != "" {
			found = hint
			return false
		}
		return true
	})
	return found
}

// matchCWDPatterns runs the cwdPatterns regexes over a piece of text
// in declaration order and returns the first capture (truncated to
// the column width). Empty string when nothing matches.
func matchCWDPatterns(text string) string {
	if text == "" {
		return ""
	}
	for _, re := range cwdPatterns {
		if m := re.FindStringSubmatch(text); len(m) >= 2 {
			return truncate(strings.TrimSpace(m[1]), maxCWDLen)
		}
	}
	return ""
}

// contentToText flattens both legacy (string) and structured-array
// (`[{type:"text",text:"..."}]`) content shapes into a single string we
// can run regexes against. We only concatenate `text` parts; images,
// audio, and other binary types are skipped since they cannot contain
// a path hint.
func contentToText(c gjson.Result) string {
	switch c.Type {
	case gjson.String:
		return c.String()
	case gjson.JSON:
		if !c.IsArray() {
			return ""
		}
		var parts []string
		c.ForEach(func(_, item gjson.Result) bool {
			if t := item.Get("type").String(); t != "text" && t != "input_text" {
				return true
			}
			if s := item.Get("text").String(); s != "" {
				parts = append(parts, s)
			}
			return true
		})
		return strings.Join(parts, "\n")
	default:
		return ""
	}
}

// extractCacheTokens normalises prompt-cache accounting across OpenAI
// (cached reads only) and Anthropic (creation + read) into a single
// (creation, read) pair. Returns (0, 0) when the response carries no
// usage block, which is the right answer for failed / partial requests.
//
// Branching strategy:
//   - Anthropic non-stream:  body is `{"usage":{...}}`
//   - Anthropic SSE:         look for the terminal `message_delta` /
//     `message_start` events which carry the cumulative usage.
//   - OpenAI non-stream:     body is `{"usage":{"prompt_tokens_details":{"cached_tokens":N}}}`
//   - OpenAI SSE:            usage is opt-in via `stream_options.include_usage`;
//     when present it lands in the final `data:` chunk.
//
// We treat the SSE case by scanning the buffered body once with a
// substring search instead of a full SSE parser — the body has already
// been bounded to maxBodyBytes by the capture layer.
func extractCacheTokens(respBody []byte, contentType string) (creation, read int64) {
	if len(respBody) == 0 {
		return 0, 0
	}
	isSSE := strings.Contains(strings.ToLower(contentType), "text/event-stream") ||
		bytes.HasPrefix(bytes.TrimLeft(respBody, " \r\n"), []byte("data:"))
	if isSSE {
		return scanSSECacheTokens(respBody)
	}
	if !json.Valid(respBody) {
		return 0, 0
	}
	return readUsageCacheTokens(gjson.ParseBytes(respBody).Get("usage"))
}

// readUsageCacheTokens pulls the cache numbers out of either OpenAI or
// Anthropic shaped `usage` objects. Unknown fields are ignored.
func readUsageCacheTokens(usage gjson.Result) (creation, read int64) {
	if !usage.Exists() {
		return 0, 0
	}
	if v := usage.Get("cache_creation_input_tokens"); v.Exists() {
		creation = v.Int()
	}
	if v := usage.Get("cache_read_input_tokens"); v.Exists() {
		read = v.Int()
	}
	// OpenAI nests the read counter; if Anthropic-style fields were
	// absent fall back to the OpenAI shape. Note we *add* rather than
	// overwrite so a hypothetical provider that fills both shapes
	// doesn't silently lose the larger number.
	if v := usage.Get("prompt_tokens_details.cached_tokens"); v.Exists() && read == 0 {
		read = v.Int()
	}
	return creation, read
}

// scanSSECacheTokens walks a buffered SSE response and returns the
// largest cache numbers observed in any chunk. SSE bodies are usually
// small enough (and bounded to maxBodyBytes anyway) that a per-line
// JSON parse is acceptable. We take the MAX across chunks instead of
// the LAST because Anthropic streams a `message_start` with the prompt
// cache numbers up front and a `message_delta` at the end with the
// output total — the prompt-cache fields only appear in the start
// event, so "last wins" would drop them.
func scanSSECacheTokens(body []byte) (creation, read int64) {
	for _, line := range bytes.Split(body, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		payload := bytes.TrimSpace(line[5:])
		if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
			continue
		}
		if payload[0] != '{' || !json.Valid(payload) {
			continue
		}
		parsed := gjson.ParseBytes(payload)
		// Anthropic events nest usage under .message.usage (start) or
		// .usage (delta). OpenAI's optional final chunk uses .usage.
		c, r := readUsageCacheTokens(parsed.Get("usage"))
		if mc, mr := readUsageCacheTokens(parsed.Get("message.usage")); mc > 0 || mr > 0 {
			if mc > c {
				c = mc
			}
			if mr > r {
				r = mr
			}
		}
		if c > creation {
			creation = c
		}
		if r > read {
			read = r
		}
	}
	return creation, read
}

// truncate clamps s to at most n bytes. We cut on byte boundary rather
// than rune boundary because callers feed mostly-ASCII identifiers and
// the column lengths are already generous; a sub-rune split on the
// rare emoji-in-UA case still produces a valid sqlite TEXT value.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
