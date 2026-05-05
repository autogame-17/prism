package claude

import (
	"encoding/json"
	"testing"
)

// TestTryParseAsClaudeContent_Tool ensures Cursor-style tool_use /
// tool_result parts survive the conversion intact instead of being
// silently dropped by the OpenAI multipart parser.
func TestTryParseAsClaudeContent_ToolResult(t *testing.T) {
	raw := []map[string]any{
		{
			"type":         "tool_result",
			"tool_use_id":  "tooluse_abc",
			"content":      []map[string]any{{"type": "text", "text": "ok"}},
		},
	}
	parts, ok := tryParseAsClaudeContent(raw)
	if !ok {
		t.Fatalf("expected ok=true for tool_result content, got false")
	}
	if len(parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(parts))
	}
	if parts[0].Type != "tool_result" {
		t.Errorf("type=%q, want tool_result", parts[0].Type)
	}
	if parts[0].ToolUseId != "tooluse_abc" {
		t.Errorf("tool_use_id=%q, want tooluse_abc", parts[0].ToolUseId)
	}
	if parts[0].Content == nil {
		t.Errorf("content was dropped, expected nested array")
	}
}

func TestTryParseAsClaudeContent_ToolUse(t *testing.T) {
	raw := []map[string]any{
		{"type": "text", "text": "I'll check that."},
		{"type": "tool_use", "id": "tooluse_xyz", "name": "read", "input": map[string]any{"path": "/x"}},
	}
	parts, ok := tryParseAsClaudeContent(raw)
	if !ok {
		t.Fatalf("expected ok=true for tool_use content")
	}
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(parts))
	}
	if parts[1].Id != "tooluse_xyz" || parts[1].Name != "read" {
		t.Errorf("tool_use fields lost: %+v", parts[1])
	}
}

// TestTryParseAsClaudeContent_PlainText keeps existing OpenAI clients on
// the legacy path. Without Anthropic-only markers, ok must be false so
// the old ParseContent flow runs unchanged.
func TestTryParseAsClaudeContent_PlainText(t *testing.T) {
	raw := []map[string]any{
		{"type": "text", "text": "hello"},
	}
	if _, ok := tryParseAsClaudeContent(raw); ok {
		t.Errorf("plain text content should not trigger Claude-native fast path")
	}
}

// TestTryParseAsClaudeContent_ImageURL covers the OpenAI image_url shape
// (used by every OpenAI-compatible client). It must NOT be classified as
// Claude-native or the conversion would skip the image-fetch step.
func TestTryParseAsClaudeContent_ImageURL(t *testing.T) {
	raw := []map[string]any{
		{"type": "image_url", "image_url": map[string]any{"url": "https://example.com/x.png"}},
	}
	if _, ok := tryParseAsClaudeContent(raw); ok {
		t.Errorf("openai image_url content should not trigger Claude-native fast path")
	}
}

// Sanity: nil + string content stay on the legacy path.
func TestTryParseAsClaudeContent_NilAndString(t *testing.T) {
	if _, ok := tryParseAsClaudeContent(nil); ok {
		t.Errorf("nil content triggered Claude path")
	}
	if _, ok := tryParseAsClaudeContent("hello"); ok {
		t.Errorf("string content triggered Claude path")
	}
}

// Round-trip safety: marshal -> tryParse -> marshal must preserve the
// Anthropic-only fields end-to-end.
func TestTryParseAsClaudeContent_RoundTrip(t *testing.T) {
	raw := []map[string]any{
		{
			"type":        "tool_result",
			"tool_use_id": "tooluse_n8ZZ",
			"content":     []map[string]any{{"type": "text", "text": "Error: foo"}},
		},
	}
	parts, ok := tryParseAsClaudeContent(raw)
	if !ok {
		t.Fatalf("setup: expected ok")
	}
	out, err := json.Marshal(parts)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(out)
	if !contains(got, `"tool_use_id":"tooluse_n8ZZ"`) {
		t.Errorf("tool_use_id missing in round-trip: %s", got)
	}
	if !contains(got, `"type":"tool_result"`) {
		t.Errorf("type missing in round-trip: %s", got)
	}
}

// TestTryParseAsClaudeContent_Thinking pins the thinking-block fix
// reported by Bugbot: without dedicated Thinking/Signature/Data fields
// on MessageContent, the fast path would unmarshal the part successfully
// (because of the type-tag trigger) but drop the actual thinking text,
// emitting a structurally-empty {"type":"thinking"} block that Bedrock
// rejects in multi-turn flows.
func TestTryParseAsClaudeContent_ThinkingRoundTrip(t *testing.T) {
	raw := []map[string]any{
		{
			"type":      "thinking",
			"thinking":  "Let me think about this step by step...",
			"signature": "abc123sig",
		},
		{
			"type": "redacted_thinking",
			"data": "encrypted-blob",
		},
	}
	parts, ok := tryParseAsClaudeContent(raw)
	if !ok {
		t.Fatalf("expected ok=true for thinking content, got false")
	}
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(parts))
	}
	if parts[0].Thinking != "Let me think about this step by step..." {
		t.Errorf("thinking text dropped: %q", parts[0].Thinking)
	}
	if parts[0].Signature != "abc123sig" {
		t.Errorf("signature dropped: %q", parts[0].Signature)
	}
	if parts[1].Data != "encrypted-blob" {
		t.Errorf("redacted_thinking data dropped: %q", parts[1].Data)
	}
	// Round-trip back to JSON and confirm the restored fields make it
	// onto the wire (this is the exact shape Bedrock receives).
	out, err := json.Marshal(parts)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	wire := string(out)
	if !contains(wire, `"thinking":"Let me think about this step by step..."`) {
		t.Errorf("thinking field missing from outbound JSON: %s", wire)
	}
	if !contains(wire, `"signature":"abc123sig"`) {
		t.Errorf("signature field missing from outbound JSON: %s", wire)
	}
	if !contains(wire, `"data":"encrypted-blob"`) {
		t.Errorf("data field missing from outbound JSON: %s", wire)
	}
}

func contains(s, sub string) bool {
	return len(sub) <= len(s) && (s == sub || stringIndex(s, sub) >= 0)
}

func stringIndex(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
