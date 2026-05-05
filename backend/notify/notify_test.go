package notify

import "testing"

// applescriptEscape must:
//   - escape backslashes BEFORE quotes so a `\"` we add for a real quote
//     isn't mangled by a follow-up `\` → `\\` rewrite
//   - turn a literal `\n` in user input into the four bytes `\\n` so
//     AppleScript renders it as the two characters \ and n, not as a
//     newline (which would split the notification into two lines)
func TestAppleScriptEscape(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{``, ``},
		{`hello`, `hello`},
		{`with "quotes"`, `with \"quotes\"`},
		{`back\slash`, `back\\slash`},
		// The headline regression — literal "\n" must NOT collapse to a
		// newline. Without the backslash-first pass this would end up
		// as `\n` in the script and AppleScript would render a line break.
		{`a\nb`, `a\\nb`},
		// A backslash directly before a quote in user input must NOT
		// turn into `\"` (which would close the string) when escaped.
		// Correct: each char gets its own double — `\\\"`.
		{`x\"y`, `x\\\"y`},
	}
	for _, tc := range cases {
		got := applescriptEscape(tc.in)
		if got != tc.want {
			t.Errorf("applescriptEscape(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
