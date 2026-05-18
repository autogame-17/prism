package bindings

import (
	"strings"
	"testing"
)

func TestRenderModelListPreview(t *testing.T) {
	cases := []struct {
		name        string
		models      []string
		want        string
		contains    []string
		notContains []string
	}{
		{
			name:   "empty list",
			models: nil,
			want:   "(provider returned empty model list)",
		},
		{
			name:   "below cap",
			models: []string{"gpt-4o"},
			want:   "(1 models: gpt-4o)",
		},
		{
			name:   "exactly at cap",
			models: []string{"a", "b", "c", "d", "e"},
			want:   "(5 models: a, b, c, d, e)",
		},
		{
			name:        "above cap truncates with count",
			models:      []string{"m1", "m2", "m3", "m4", "m5", "m6", "m7"},
			contains:    []string{"7 models", "e.g. m1, m2, m3, m4, m5"},
			notContains: []string{"m6", "m7"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := renderModelListPreview(tc.models)
			if tc.want != "" && got != tc.want {
				t.Fatalf("preview mismatch:\n got: %q\nwant: %q", got, tc.want)
			}
			for _, sub := range tc.contains {
				if !strings.Contains(got, sub) {
					t.Errorf("preview %q missing substring %q", got, sub)
				}
			}
			for _, sub := range tc.notContains {
				if strings.Contains(got, sub) {
					t.Errorf("preview %q should not contain %q (cap exceeded)", got, sub)
				}
			}
		})
	}
}
