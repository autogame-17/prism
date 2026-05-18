package identity

import (
	"os"
	"path/filepath"
	"testing"
)

// TestInit_FreshAndStable verifies:
//  1. A fresh data dir gets a new id written to disk.
//  2. A second Init call against the same dir returns the same id.
//  3. The id is exactly idBytes*2 hex chars.
func TestInit_FreshAndStable(t *testing.T) {
	dir := t.TempDir()
	id1, err := Init(dir)
	if err != nil {
		t.Fatalf("first init: %v", err)
	}
	if !isValid(id1) {
		t.Fatalf("invalid id: %q", id1)
	}
	id2, err := Init(dir)
	if err != nil {
		t.Fatalf("second init: %v", err)
	}
	if id2 != id1 {
		t.Fatalf("id not stable across calls: %q vs %q", id1, id2)
	}
	if got := Get(); got != id1 {
		t.Fatalf("Get() = %q, want %q", got, id1)
	}
}

// TestInit_RotatesOnCorruption ensures a malformed .device_id is
// transparently rotated rather than blocking boot.
func TestInit_RotatesOnCorruption(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, fileName), []byte("not-hex!"), 0o600); err != nil {
		t.Fatalf("seed corrupt file: %v", err)
	}
	id, err := Init(dir)
	if err != nil {
		t.Fatalf("init after corruption: %v", err)
	}
	if !isValid(id) {
		t.Fatalf("invalid id after rotation: %q", id)
	}
}
