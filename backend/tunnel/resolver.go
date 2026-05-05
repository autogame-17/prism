// Package tunnel wraps the cloudflared binary. The actual binary for the
// target platform is injected via SetEmbeddedBinary from package main (which
// can reach resources/cloudflared/<os>-<arch>/cloudflared via go:embed).
//
// During `wails dev` and on platforms where a vendored binary is missing,
// ResolveBinary() falls back to looking up `cloudflared` in PATH so developers
// can iterate without running scripts/fetch_cloudflared.sh first.
package tunnel

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
)

// binaryBytes is populated via SetEmbeddedBinary at init time.
var binaryBytes []byte

// binaryName is the filesystem name the binary should have on disk.
var binaryName = "cloudflared"

// SetEmbeddedBinary registers bytes from go:embed at the caller side (main).
// Pass the appropriate filename for Windows.
func SetEmbeddedBinary(b []byte, name string) {
	binaryBytes = b
	if name != "" {
		binaryName = name
	}
}

// ResolveBinary materialises the cloudflared executable, writing the embedded
// bytes to cacheDir if necessary. If no bytes are vendored, it tries to find
// cloudflared on PATH instead. The returned path is ready to exec.
func ResolveBinary(cacheDir string) (string, error) {
	if len(binaryBytes) > 0 {
		dest := filepath.Join(cacheDir, binaryName)
		if err := os.MkdirAll(cacheDir, 0o755); err != nil {
			return "", err
		}
		if !sameContents(dest, binaryBytes) {
			if err := os.WriteFile(dest, binaryBytes, 0o755); err != nil {
				return "", err
			}
		}
		return dest, nil
	}
	if p, err := exec.LookPath("cloudflared"); err == nil {
		return p, nil
	}
	// Last resort: well-known install locations on macOS.
	for _, p := range []string{
		"/opt/homebrew/bin/cloudflared",
		"/usr/local/bin/cloudflared",
	} {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", errors.New("cloudflared binary not found (not embedded and not on PATH)")
}

func sameContents(path string, want []byte) bool {
	st, err := os.Stat(path)
	if err != nil || st.Size() != int64(len(want)) {
		return false
	}
	got, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
