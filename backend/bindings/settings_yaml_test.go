package bindings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeNestedStringFieldToYAML and readNestedStringFromYAML are critical:
// they touch user-edited prism.yaml without bringing in a full YAML
// parser. The tests below pin down upsert / delete / preservation
// behaviour so we can't silently corrupt configs across releases.

func writeFile(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "prism.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatalf("seed yaml: %v", err)
	}
	return p
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read yaml: %v", err)
	}
	return string(b)
}

func TestNestedYAML_AppendsParentWhenMissing(t *testing.T) {
	p := writeFile(t, "gin_mode: \"release\"\nlisten_addr: \"127.0.0.1:39527\"\n")
	if err := writeNestedStringFieldToYAML(p, "cloudflare", "tunnel_token", "abc.def"); err != nil {
		t.Fatalf("write: %v", err)
	}
	got := readFile(t, p)
	if !strings.Contains(got, "cloudflare:\n  tunnel_token: \"abc.def\"\n") {
		t.Errorf("parent block missing or malformed:\n%s", got)
	}
	if !strings.HasPrefix(got, "gin_mode: \"release\"") {
		t.Errorf("pre-existing top-level keys disturbed:\n%s", got)
	}
}

func TestNestedYAML_UpsertsExistingChild(t *testing.T) {
	p := writeFile(t, `gin_mode: "release"

cloudflare:
  tunnel_token: "old"
  tunnel_hostname: "old.example.com"
`)
	if err := writeNestedStringFieldToYAML(p, "cloudflare", "tunnel_token", "new.token"); err != nil {
		t.Fatalf("write: %v", err)
	}
	got := readFile(t, p)
	if !strings.Contains(got, `tunnel_token: "new.token"`) {
		t.Errorf("token not updated:\n%s", got)
	}
	if !strings.Contains(got, `tunnel_hostname: "old.example.com"`) {
		t.Errorf("sibling key got dropped:\n%s", got)
	}
}

func TestNestedYAML_PreservesIndentation(t *testing.T) {
	// The user picked 4-space indentation; we must follow suit when
	// inserting a new child rather than forcing 2-space which would
	// create a malformed block.
	p := writeFile(t, "cloudflare:\n    tunnel_hostname: \"prism.example.com\"\n")
	if err := writeNestedStringFieldToYAML(p, "cloudflare", "tunnel_token", "tok"); err != nil {
		t.Fatalf("write: %v", err)
	}
	got := readFile(t, p)
	if !strings.Contains(got, "    tunnel_token: \"tok\"") {
		t.Errorf("indentation not preserved:\n%s", got)
	}
}

func TestNestedYAML_RemovesChildOnEmpty(t *testing.T) {
	p := writeFile(t, `cloudflare:
  tunnel_token: "abc"
  tunnel_hostname: "x"
`)
	if err := writeNestedStringFieldToYAML(p, "cloudflare", "tunnel_token", ""); err != nil {
		t.Fatalf("write: %v", err)
	}
	got := readFile(t, p)
	if strings.Contains(got, "tunnel_token") {
		t.Errorf("token line should be gone:\n%s", got)
	}
	if !strings.Contains(got, `tunnel_hostname: "x"`) {
		t.Errorf("sibling dropped:\n%s", got)
	}
}

func TestNestedYAML_RoundTrip(t *testing.T) {
	p := writeFile(t, "")
	if err := writeNestedStringFieldToYAML(p, "cloudflare", "tunnel_token", "x"); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := readNestedStringFromYAML(p, "cloudflare", "tunnel_token")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got != "x" {
		t.Errorf("got %q, want %q", got, "x")
	}
}

func TestNestedYAML_ReadMissingReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	v, err := readNestedStringFromYAML(filepath.Join(dir, "missing.yaml"), "cloudflare", "tunnel_token")
	if err != nil {
		t.Fatalf("read of missing file: %v", err)
	}
	if v != "" {
		t.Errorf("expected empty for missing file, got %q", v)
	}
}

// Make sure nested writes don't break the flat helper used by
// listen_addr — writing both must keep both readable side by side.
func TestNestedAndFlat_Coexist(t *testing.T) {
	p := writeFile(t, "")
	if err := writeStringFieldToYAML(p, "listen_addr", "127.0.0.1:39527"); err != nil {
		t.Fatalf("flat write: %v", err)
	}
	if err := writeNestedStringFieldToYAML(p, "cloudflare", "tunnel_token", "tok"); err != nil {
		t.Fatalf("nested write: %v", err)
	}
	got := readFile(t, p)
	if !strings.Contains(got, `listen_addr: "127.0.0.1:39527"`) {
		t.Errorf("flat key missing:\n%s", got)
	}
	if !strings.Contains(got, `tunnel_token: "tok"`) {
		t.Errorf("nested key missing:\n%s", got)
	}
}
