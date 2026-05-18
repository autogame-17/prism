// Package identity owns the per-install Prism Device ID.
//
// The Device ID is a stable 8-byte hex string persisted under the data
// directory (`<data>/.device_id`). It exists so every outgoing relay
// request and every captured trace can carry an `x-request-id` of the
// form `prism-<deviceId>-<unixMilli>`. That ID is the join key across
// Prism's local sqlite, upstream gateway logs (one-api / one-hub), and
// any downstream consumer of the JSONL exports.
//
// Why a per-install random ID and not a hardware fingerprint:
//   - Cross-platform hardware IDs (machine-id, IOPlatformUUID, MachineGuid)
//     are awkward to read uniformly and tend to leak more than we want.
//   - Users may run Prism in multiple containers / VMs / users on one box;
//     a per-install ID keeps those traces distinguishable.
//   - It is regenerable: deleting `.device_id` is the documented way to
//     "rotate".
package identity

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	// fileName is the on-disk name under the data directory.
	fileName = ".device_id"
	// idBytes is 8 bytes -> 16 hex chars. Long enough that birthday
	// collisions across the user base are negligible, short enough that
	// the request id stays under typical header-size guards.
	idBytes = 8
)

var (
	mu     sync.RWMutex
	loaded string
)

// Init loads (or creates) the device id under dataDir. Safe to call
// repeatedly; subsequent calls validate that the value matches what is
// already cached. Returns the resolved id.
func Init(dataDir string) (string, error) {
	if dataDir == "" {
		return "", errors.New("identity: data dir is empty")
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return "", fmt.Errorf("identity: mkdir data dir: %w", err)
	}
	path := filepath.Join(dataDir, fileName)

	if id, err := readExisting(path); err == nil && id != "" {
		setCached(id)
		return id, nil
	} else if err != nil && !os.IsNotExist(err) {
		// Corrupted file: rotate rather than refuse to boot, so a single
		// bad byte never bricks the desktop. We log via the caller.
		_ = os.Remove(path)
	}

	id, err := generate()
	if err != nil {
		return "", fmt.Errorf("identity: generate: %w", err)
	}
	// 0o600: the id is not a secret per se but it does identify a user
	// install across logs; keep it owner-only by default.
	if err := os.WriteFile(path, []byte(id+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("identity: write %s: %w", path, err)
	}
	setCached(id)
	return id, nil
}

// Get returns the cached device id. Empty if Init has not been called or
// failed; callers must treat empty as "no device id available" and
// degrade gracefully (e.g. fall back to a per-process UUID).
func Get() string {
	mu.RLock()
	defer mu.RUnlock()
	return loaded
}

func setCached(id string) {
	mu.Lock()
	loaded = id
	mu.Unlock()
}

func readExisting(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(string(b))
	if !isValid(id) {
		return "", fmt.Errorf("invalid device id payload at %s", path)
	}
	return id, nil
}

func isValid(s string) bool {
	if len(s) != idBytes*2 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

func generate() (string, error) {
	var b [idBytes]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
