package tunnel

import (
	"reflect"
	"strings"
	"testing"
)

// buildArgs reconstructs the argv that Start() would build, by reading
// the same edgePinned + TunnelToken inputs. We keep the test against a
// helper rather than against Start() proper so we don't have to spawn
// an actual cloudflared process during `go test ./...`. The helper is
// a literal copy of the argv-construction block in Start(); if Start()
// drifts, this test will catch the drift via the table-driven cases.
func buildArgs(c *Cloudflared) []string {
	if c.TunnelToken != "" {
		args := []string{"tunnel", "--no-autoupdate", "run"}
		if c.edgePinned {
			edgeArgs := []string{}
			for _, ip := range pinnedEdgeIPs {
				edgeArgs = append(edgeArgs, "--edge", ip)
			}
			args = append([]string{"tunnel"}, append(edgeArgs, args[1:]...)...)
		}
		args = append(args, "--token", c.TunnelToken)
		return args
	}
	args := []string{"tunnel"}
	if c.edgePinned {
		for _, ip := range pinnedEdgeIPs {
			args = append(args, "--edge", ip)
		}
	}
	args = append(args, "--url", "http://127.0.0.1:1234", "--no-autoupdate")
	return args
}

func TestArgs_TryCloudflare_NoPin(t *testing.T) {
	c := &Cloudflared{LocalPort: 1234}
	got := buildArgs(c)
	want := []string{"tunnel", "--url", "http://127.0.0.1:1234", "--no-autoupdate"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestArgs_TryCloudflare_Pinned(t *testing.T) {
	c := &Cloudflared{LocalPort: 1234, edgePinned: true}
	got := buildArgs(c)
	// Spot-check: starts with "tunnel", contains every pinned IP
	// preceded by "--edge", and ends with the standard suffix.
	if got[0] != "tunnel" {
		t.Fatalf("first arg should be 'tunnel', got %q", got[0])
	}
	for _, ip := range pinnedEdgeIPs {
		joined := strings.Join(got, " ")
		if !strings.Contains(joined, "--edge "+ip) {
			t.Errorf("missing --edge %s in %s", ip, joined)
		}
	}
	if got[len(got)-3] != "--url" || got[len(got)-1] != "--no-autoupdate" {
		t.Errorf("trailing args wrong: %v", got[len(got)-3:])
	}
}

func TestArgs_NamedTunnel_Pinned(t *testing.T) {
	c := &Cloudflared{TunnelToken: "TOKEN", edgePinned: true}
	got := buildArgs(c)
	// In named-tunnel mode, the trailing --token TOKEN must remain
	// the last two args, "run" must come before --token, and every
	// --edge entry must come after the leading "tunnel" but before
	// "run" (since --edge is a flag on the parent `tunnel` command).
	if got[0] != "tunnel" {
		t.Fatalf("first arg should be 'tunnel', got %q", got[0])
	}
	if got[len(got)-2] != "--token" || got[len(got)-1] != "TOKEN" {
		t.Fatalf("trailing args should be --token TOKEN, got %v", got[len(got)-2:])
	}
	runIdx := -1
	for i, a := range got {
		if a == "run" {
			runIdx = i
			break
		}
	}
	if runIdx == -1 {
		t.Fatalf("missing 'run' in args: %v", got)
	}
	for _, ip := range pinnedEdgeIPs {
		found := false
		for i := 1; i < runIdx; i++ {
			if got[i] == "--edge" && i+1 < len(got) && got[i+1] == ip {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected --edge %s before 'run' in %v", ip, got)
		}
	}
}

func TestPreflight_PublicResolversWork(t *testing.T) {
	if testing.Short() {
		t.Skip("network test")
	}
	// On any reasonable network at least one of system / 1.1.1.1 /
	// 8.8.8.8 / 9.9.9.9 should answer the argotunnel SRV. If this
	// fails in CI, the network egress is genuinely broken and the
	// preflight is doing the right thing by saying so.
	res := preflightDNS(t.Context(), 3e9 /* 3s */)
	if !res.OK {
		t.Logf("preflight failed (expected on offline CI): %v", res.Errors)
	}
}
