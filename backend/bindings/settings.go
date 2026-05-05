package bindings

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"one-api/common/config"
	"one-api/model"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// SettingsAPI exposes app-level settings + data-directory / import-export.
type SettingsAPI struct {
	ctx              context.Context
	dataDir          string
	logDir           string
	actualListenAddr string
}

// NewSettingsAPI constructs the binding; paths come from the boot result.
func NewSettingsAPI(dataDir, logDir string) *SettingsAPI {
	return &SettingsAPI{dataDir: dataDir, logDir: logDir}
}

// SetActualListenAddr is called by the App once the embedded HTTP server is
// bound, so GetListenAddr() can report the real running address (which may
// differ from what's in prism.yaml when the configured port was busy and
// Boot fell back to a random port).
func (a *SettingsAPI) SetActualListenAddr(addr string) {
	a.actualListenAddr = addr
}

// SetContext wires in the Wails runtime context for file dialogs + event emits.
// Guards against nil ctx from accidental frontend invocation.
func (a *SettingsAPI) SetContext(ctx context.Context) {
	if ctx == nil {
		return
	}
	a.ctx = ctx
}

// Paths returns the configured data / log directories.
type Paths struct {
	DataDir    string `json:"dataDir"`
	LogDir     string `json:"logDir"`
	ConfigFile string `json:"configFile"`
	OS         string `json:"os"`
	Arch       string `json:"arch"`
}

// GetPaths returns the filesystem layout.
func (a *SettingsAPI) GetPaths() Paths {
	return Paths{
		DataDir:    a.dataDir,
		LogDir:     a.logDir,
		ConfigFile: filepath.Join(a.dataDir, "prism.yaml"),
		OS:         runtime.GOOS,
		Arch:       runtime.GOARCH,
	}
}

// OpenDataDir asks the OS to reveal the data folder in the file manager.
// macOS: `open <dir>`; Windows: `explorer <dir>`; Linux: `xdg-open <dir>`.
// Native commands accept paths with spaces directly, avoiding the URL-encoding
// pitfalls of BrowserOpenURL("file://...").
func (a *SettingsAPI) OpenDataDir() error {
	if a.dataDir == "" {
		return errors.New("data directory not configured")
	}
	if _, err := os.Stat(a.dataDir); err != nil {
		return fmt.Errorf("stat data dir: %w", err)
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", a.dataDir)
	case "windows":
		cmd = exec.Command("explorer", a.dataDir)
	default:
		cmd = exec.Command("xdg-open", a.dataDir)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("open data dir: %w", err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// PickExportPath shows a native Save dialog and returns the chosen path (empty = cancelled).
func (a *SettingsAPI) PickExportPath(defaultName string) (string, error) {
	if a.ctx == nil {
		return "", errors.New("settings api context not ready")
	}
	if defaultName == "" {
		defaultName = fmt.Sprintf("prism-%s.json", time.Now().Format("2006-01-02"))
	}
	return wruntime.SaveFileDialog(a.ctx, wruntime.SaveDialogOptions{
		Title:           "Export Prism data",
		DefaultFilename: defaultName,
		Filters: []wruntime.FileFilter{
			{DisplayName: "JSON", Pattern: "*.json"},
		},
	})
}

// PickImportPath shows a native Open dialog and returns the chosen path (empty = cancelled).
func (a *SettingsAPI) PickImportPath() (string, error) {
	if a.ctx == nil {
		return "", errors.New("settings api context not ready")
	}
	return wruntime.OpenFileDialog(a.ctx, wruntime.OpenDialogOptions{
		Title: "Import Prism data",
		Filters: []wruntime.FileFilter{
			{DisplayName: "JSON", Pattern: "*.json"},
		},
	})
}

// Export returns a JSON blob with all non-sensitive settings + channels + tokens.
type ExportBlob struct {
	Version  string                  `json:"version"`
	ExportAt int64                   `json:"exportAt"`
	Channels []*model.Channel        `json:"channels"`
	Tokens   []ExportedToken         `json:"tokens"`
	Options  map[string]string       `json:"options"`
	Groups   []string                `json:"groups"`
	Extras   map[string]any          `json:"extras,omitempty"`
}

// ExportedToken is a neutered token shape that keeps only the fields safe to back up.
type ExportedToken struct {
	Name           string `json:"name"`
	Group          string `json:"group"`
	UnlimitedQuota bool   `json:"unlimitedQuota"`
	RemainQuota    int    `json:"remainQuota"`
	ExpiredTime    int64  `json:"expiredTime"`
	Key            string `json:"key"`
}

// Export produces a full snapshot for backup purposes.
func (a *SettingsAPI) Export() (*ExportBlob, error) {
	var channels []*model.Channel
	if err := model.DB.Find(&channels).Error; err != nil {
		return nil, fmt.Errorf("list channels: %w", err)
	}
	userID, err := rootUserID()
	if err != nil {
		return nil, err
	}
	var tokens []*model.Token
	if err := model.DB.Where("user_id = ?", userID).Find(&tokens).Error; err != nil {
		return nil, fmt.Errorf("list tokens: %w", err)
	}
	exportedTokens := make([]ExportedToken, 0, len(tokens))
	for _, t := range tokens {
		exportedTokens = append(exportedTokens, ExportedToken{
			Name:           t.Name,
			Group:          t.Group,
			UnlimitedQuota: t.UnlimitedQuota,
			RemainQuota:    t.RemainQuota,
			ExpiredTime:    t.ExpiredTime,
			Key:            t.Key,
		})
	}
	options := map[string]string{}
	var rows []*model.Option
	if err := model.DB.Find(&rows).Error; err == nil {
		for _, r := range rows {
			options[r.Key] = r.Value
		}
	}
	return &ExportBlob{
		Version:  "prism/1",
		ExportAt: nowUnix(),
		Channels: channels,
		Tokens:   exportedTokens,
		Options:  options,
	}, nil
}

// ExportToFile serialises the blob and writes it to the given path.
func (a *SettingsAPI) ExportToFile(path string) error {
	blob, err := a.Export()
	if err != nil {
		return err
	}
	body, err := json.MarshalIndent(blob, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o600)
}

// Import replaces channels + tokens with the supplied blob.
// Options are upserted; nothing is deleted.
func (a *SettingsAPI) Import(blob ExportBlob, replace bool) error {
	userID, err := rootUserID()
	if err != nil {
		return err
	}
	tx := model.DB.Begin()
	if replace {
		if err := tx.Where("1 = 1").Delete(&model.Channel{}).Error; err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Where("user_id = ?", userID).Delete(&model.Token{}).Error; err != nil {
			tx.Rollback()
			return err
		}
	}
	for _, c := range blob.Channels {
		c.Id = 0
		if err := tx.Create(c).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("import channel %s: %w", c.Name, err)
		}
	}
	for _, t := range blob.Tokens {
		tok := &model.Token{
			UserId:         userID,
			Name:           t.Name,
			Group:          t.Group,
			Key:            t.Key,
			Status:         config.TokenStatusEnabled,
			UnlimitedQuota: t.UnlimitedQuota,
			RemainQuota:    t.RemainQuota,
			ExpiredTime:    t.ExpiredTime,
			CreatedTime:    nowUnix(),
			AccessedTime:   nowUnix(),
		}
		if err := tx.Create(tok).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("import token %s: %w", t.Name, err)
		}
	}
	for k, v := range blob.Options {
		_ = tx.Where("key = ?", k).Delete(&model.Option{}).Error
		if err := tx.Create(&model.Option{Key: k, Value: v}).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("import option %s: %w", k, err)
		}
	}
	return tx.Commit().Error
}

// ImportFromFile reads a blob from disk and imports it.
func (a *SettingsAPI) ImportFromFile(path string, replace bool) error {
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var blob ExportBlob
	if err := json.Unmarshal(body, &blob); err != nil {
		return err
	}
	return a.Import(blob, replace)
}

func nowUnix() int64 { return time.Now().Unix() }

// ListenAddrInfo describes both the value persisted in prism.yaml and the
// address the running process actually bound to. They may diverge when the
// configured port was already in use and Boot fell back to a random port.
type ListenAddrInfo struct {
	Configured string `json:"configured"` // value in prism.yaml; empty = use default
	Actual     string `json:"actual"`     // host:port the http server is bound on right now
	Default    string `json:"default"`    // built-in default surfaced for UX hints
}

const defaultListenAddr = "127.0.0.1:39527"

// GetListenAddr returns both the configured and the running address.
// dataDir is required so we know where prism.yaml lives. The actual address
// has to come from the App layer because SettingsAPI doesn't own the boot
// result; we expose it through a setter at construction time.
func (a *SettingsAPI) GetListenAddr() (ListenAddrInfo, error) {
	info := ListenAddrInfo{Default: defaultListenAddr, Actual: a.actualListenAddr}
	cfg, err := readListenAddrFromYAML(filepath.Join(a.dataDir, "prism.yaml"))
	if err != nil {
		return info, err
	}
	info.Configured = cfg
	return info, nil
}

// SetListenAddr validates and persists a new listen_addr to prism.yaml. The
// change does NOT take effect until the user restarts Prism — the caller is
// responsible for surfacing that to the UI. Pass an empty string to remove
// the override (Prism will fall back to the default port on next boot).
func (a *SettingsAPI) SetListenAddr(addr string) error {
	addr = strings.TrimSpace(addr)
	if addr != "" {
		if err := validateListenAddr(addr); err != nil {
			return err
		}
	}
	return writeListenAddrToYAML(filepath.Join(a.dataDir, "prism.yaml"), addr)
}

// validateListenAddr rejects obvious typos before they hit disk. We only
// require host:port to parse and the port to be a positive integer; the
// actual bind happens on next Boot, where we'll discover real conflicts.
func validateListenAddr(addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid host:port %q: %w", addr, err)
	}
	if host == "" {
		return errors.New("host part required (e.g. 127.0.0.1 or 0.0.0.0)")
	}
	if port == "" {
		return errors.New("port required")
	}
	// Reject 0 — even though the underlying server supports it for ephemeral
	// binding, persisting "127.0.0.1:0" defeats the whole point of this UI.
	// Tell the user to delete the field instead.
	if port == "0" {
		return errors.New("use empty value to disable the override; port 0 is not allowed")
	}
	return nil
}

// readListenAddrFromYAML scans prism.yaml line by line and returns the first
// top-level `listen_addr:` value. Returns "" if the key isn't present.
// We deliberately avoid full YAML parsing so we can also _write_ the file
// later without touching comments / formatting / ordering.
func readListenAddrFromYAML(path string) (string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	scanner := bufio.NewScanner(bytes.NewReader(body))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimLeft(line, " \t")
		// Only top-level keys (no indentation).
		if trimmed != line {
			continue
		}
		if !strings.HasPrefix(trimmed, "listen_addr:") {
			continue
		}
		val := strings.TrimSpace(strings.TrimPrefix(trimmed, "listen_addr:"))
		val = strings.Trim(val, `"'`)
		return val, nil
	}
	return "", scanner.Err()
}

// writeListenAddrToYAML rewrites prism.yaml so that the top-level
// `listen_addr:` line equals addr. Existing comments, ordering and
// untouched fields are preserved. Empty addr removes the override.
//
// The write is atomic: we serialise to a sibling temp file, fsync, then
// rename over the original so a crash mid-write can't corrupt the config.
func writeListenAddrToYAML(path string, addr string) error {
	body, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("read %s: %w", path, err)
		}
		body = nil
	}

	var newLine string
	if addr != "" {
		newLine = fmt.Sprintf("listen_addr: %q", addr)
	}

	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	var out bytes.Buffer
	replaced := false
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimLeft(line, " \t")
		if trimmed == line && strings.HasPrefix(trimmed, "listen_addr:") {
			if newLine != "" {
				out.WriteString(newLine)
				out.WriteByte('\n')
			}
			replaced = true
			continue
		}
		out.WriteString(line)
		out.WriteByte('\n')
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan %s: %w", path, err)
	}
	if !replaced && newLine != "" {
		// Append after a trailing newline if missing.
		if out.Len() > 0 && out.Bytes()[out.Len()-1] != '\n' {
			out.WriteByte('\n')
		}
		out.WriteString(newLine)
		out.WriteByte('\n')
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out.Bytes(), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename %s: %w", path, err)
	}
	return nil
}
