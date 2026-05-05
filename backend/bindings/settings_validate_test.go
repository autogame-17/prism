package bindings

import "testing"

// validateListenAddr is the last line of defence before we persist a
// listen_addr to prism.yaml — a malformed value here would otherwise
// silently degrade the "stable local port" feature on next boot
// (net.Listen would fail, the fallback would pick a random port, and
// the user would think their pinned URL still works).
func TestValidateListenAddr(t *testing.T) {
	cases := []struct {
		name    string
		addr    string
		wantErr bool
	}{
		{"valid loopback", "127.0.0.1:39527", false},
		{"valid wildcard", "0.0.0.0:8080", false},
		{"valid max port", "127.0.0.1:65535", false},
		{"valid min port", "127.0.0.1:1", false},

		{"missing host", ":39527", true},
		{"missing port", "127.0.0.1:", true},
		{"missing colon", "127.0.0.1", true},
		{"port zero rejected", "127.0.0.1:0", true},
		// Bugbot-flagged regressions: non-numeric and out-of-range
		// ports used to slip past validation and fail silently on
		// next net.Listen.
		{"non-numeric port", "127.0.0.1:abc", true},
		{"port too large", "127.0.0.1:99999", true},
		{"port negative-looking", "127.0.0.1:-1", true},
		{"port hex-looking", "127.0.0.1:0x80", true},
		{"port with trailing junk", "127.0.0.1:80x", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateListenAddr(tc.addr)
			if tc.wantErr && err == nil {
				t.Errorf("validateListenAddr(%q): expected error, got nil", tc.addr)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("validateListenAddr(%q): unexpected error: %v", tc.addr, err)
			}
		})
	}
}
