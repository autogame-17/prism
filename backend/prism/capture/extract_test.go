package capture

import (
	"strings"
	"testing"
)

// TestExtractCWD covers the path hints emitted by the clients that
// actually flow through Prism today: Cursor's `Workspace Path:`,
// Claude Code's `Primary working directory:`, Aider's `cwd:`, and the
// `<cwd>...</cwd>` shape used by some hand-rolled scripts. The check
// is deliberately strict on the captured path because false positives
// (matching a path mentioned in tool output) are worse than misses.
func TestExtractCWD(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "cursor workspace path",
			body: `{"messages":[{"role":"system","content":"You are an AI. Workspace Path: /home/me/project\nMore text."}]}`,
			want: "/home/me/project",
		},
		{
			name: "claude code primary working directory in messages",
			body: `{"messages":[{"role":"system","content":[{"type":"text","text":"- Primary working directory: /home/AUROR\n- Other rule"}]}]}`,
			want: "/home/AUROR",
		},
		{
			name: "anthropic system field, string form",
			body: `{"system":"You are an assistant.\n- Primary working directory: /home/AUROR/from-system\n","messages":[{"role":"user","content":"hi"}]}`,
			want: "/home/AUROR/from-system",
		},
		{
			name: "anthropic system field, structured-array form",
			body: `{"system":[{"type":"text","text":"- Primary working directory: /workspace/sysarr"}],"messages":[{"role":"user","content":"hi"}]}`,
			want: "/workspace/sysarr",
		},
		{
			name: "system field wins over messages when both have hints",
			body: `{"system":"Primary working directory: /from/system","messages":[{"role":"user","content":"Workspace Path: /from/messages"}]}`,
			want: "/from/system",
		},
		{
			name: "current working directory variant",
			body: `{"messages":[{"role":"user","content":"current working directory: /tmp/work"}]}`,
			want: "/tmp/work",
		},
		{
			name: "current working directory is variant",
			body: `{"messages":[{"role":"system","content":"the current working directory is /var/data right now"}]}`,
			want: "/var/data",
		},
		{
			name: "aider cwd shorthand",
			body: `{"messages":[{"role":"developer","content":"cwd: /home/user/repo"}]}`,
			want: "/home/user/repo",
		},
		{
			name: "xml cwd tag",
			body: `{"messages":[{"role":"system","content":"<cwd>/srv/app</cwd>"}]}`,
			want: "/srv/app",
		},
		{
			name: "windows drive letter (forward slash form)",
			body: `{"messages":[{"role":"system","content":"Workspace Path: C:/Users/me/Project"}]}`,
			want: "C:/Users/me/Project",
		},
		{
			name: "ignored when only assistant emits the hint",
			body: `{"messages":[{"role":"assistant","content":"I think cwd: /not/real should be ignored"}]}`,
			want: "",
		},
		{
			name: "ignored when role is tool",
			body: `{"messages":[{"role":"tool","content":"Primary working directory: /no"}]}`,
			want: "",
		},
		{
			name: "no messages array returns empty",
			body: `{"prompt":"Workspace Path: /never/scan"}`,
			want: "",
		},
		{
			name: "empty body returns empty",
			body: ``,
			want: "",
		},
		{
			name: "stops at quote so json escaping does not leak",
			body: `{"messages":[{"role":"system","content":"Workspace Path: /home/me/project\"trailing"}]}`,
			want: "/home/me/project",
		},
		{
			name: "primary works inside structured array content",
			body: `{"messages":[{"role":"system","content":[{"type":"text","text":"- Primary working directory: /workspace/code"}]}]}`,
			want: "/workspace/code",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractCWD([]byte(tt.body))
			if got != tt.want {
				t.Fatalf("extractCWD = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestExtractCWD_TruncatesLongPaths guards against an oversized path
// blowing past the 512-char column width. The truncation is silent on
// purpose: we'd rather store a clipped value than reject the row.
func TestExtractCWD_TruncatesLongPaths(t *testing.T) {
	long := "/" + strings.Repeat("a", maxCWDLen+50)
	body := `{"messages":[{"role":"system","content":"Workspace Path: ` + long + `"}]}`
	got := extractCWD([]byte(body))
	if len(got) != maxCWDLen {
		t.Fatalf("len(extractCWD) = %d, want %d", len(got), maxCWDLen)
	}
	if !strings.HasPrefix(got, "/aaaa") {
		t.Fatalf("truncated path lost prefix: %q", got[:10])
	}
}
