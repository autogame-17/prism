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
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"one-api/common/config"
	"one-api/model"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// prismYAMLMu serialises every write to prism.yaml so two concurrent
// settings calls (SetListenAddr + SetCloudflareTunnel, or two of the
// same kind from a debounced UI) can't lose each other's edits via the
// classic read-modify-write race. The atomic tmp+rename only protects
// against a partial file from a single writer; nothing protects two
// writers reading the pre-edit state in parallel and stomping each
// other's tmp file. The mutex is process-wide because every binding
// here writes the same prism.yaml under dataDir.
var prismYAMLMu sync.Mutex

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

// validateListenAddr rejects obvious typos before they hit disk. We
// require host:port to parse and the port to be a numeric value in
// 1..65535. The actual bind happens on next Boot where we'll discover
// real conflicts (and gracefully fall back to a random port), but
// non-numeric / out-of-range ports would always fail there silently —
// catching them here keeps the "stable local port" feature from
// degrading without the user noticing.
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
	// Reject 0 — even though the underlying server supports it for
	// ephemeral binding, persisting "127.0.0.1:0" defeats the whole
	// point of this UI. Tell the user to delete the field instead.
	if port == "0" {
		return errors.New("use empty value to disable the override; port 0 is not allowed")
	}
	// strconv.ParseUint with bitSize=16 enforces 1..65535 in one shot:
	// non-numeric input fails the parse, anything > 65535 overflows
	// uint16. The "0" case is already handled above, so we don't have
	// to worry about ParseUint accepting it here.
	if _, err := strconv.ParseUint(port, 10, 16); err != nil {
		return fmt.Errorf("port %q must be a number in 1..65535", port)
	}
	return nil
}

// unquoteYAMLScalar undoes the encoding our writers apply when they
// emit a value with fmt.Sprintf("%q", ...). Without this step a
// hostname or token containing a backslash or quote would round-trip
// corrupted: the writer turns `a"b` into the on-disk bytes `"a\"b"`,
// and a naive reader that just strips outer quotes would hand the
// rest of the system `a\"b` instead of the original `a"b`.
//
// For values that aren't double-quoted on disk (user-edited plain
// scalars, or single-quoted forms) we fall back to the looser
// strip-outer-quotes behaviour so we don't break hand-written configs.
func unquoteYAMLScalar(val string) string {
	val = strings.TrimSpace(val)
	if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
		if unquoted, err := strconv.Unquote(val); err == nil {
			return unquoted
		}
		// Malformed quoted form — fall through to the looser strip
		// rather than discard whatever the user has on disk.
	}
	return strings.Trim(val, `"'`)
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
		val := strings.TrimPrefix(trimmed, "listen_addr:")
		return unquoteYAMLScalar(val), nil
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
	return writeStringFieldToYAML(path, "listen_addr", addr)
}

// CloudflareTunnelInfo describes the named-tunnel configuration. Token
// is intentionally NOT echoed back to the UI in cleartext; we only
// signal whether one is set so the user can decide whether to clear or
// rotate it. Hostname round-trips fine because it's user-public anyway
// (it's the URL their clients are calling).
type CloudflareTunnelInfo struct {
	HasToken bool   `json:"hasToken"`
	Hostname string `json:"hostname"`
}

// GetCloudflareTunnel reads the named-tunnel config out of prism.yaml.
func (a *SettingsAPI) GetCloudflareTunnel() (CloudflareTunnelInfo, error) {
	path := filepath.Join(a.dataDir, "prism.yaml")
	token, err := readNestedStringFromYAML(path, "cloudflare", "tunnel_token")
	if err != nil {
		return CloudflareTunnelInfo{}, err
	}
	host, err := readNestedStringFromYAML(path, "cloudflare", "tunnel_hostname")
	if err != nil {
		return CloudflareTunnelInfo{}, err
	}
	return CloudflareTunnelInfo{HasToken: token != "", Hostname: host}, nil
}

// SetCloudflareTunnel persists a named-tunnel token + public hostname
// to prism.yaml. Pass empty token to clear the override (Prism will fall
// back to trycloudflare on next start). Hostname must be a bare host
// (e.g. "prism.example.com"), no scheme.
//
// Like other settings writes, the change does NOT take effect until the
// user restarts Prism, because the cloudflared subprocess captures the
// token at Start time. The caller (UI) is responsible for surfacing the
// restart hint.
func (a *SettingsAPI) SetCloudflareTunnel(token, hostname string) error {
	token = strings.TrimSpace(token)
	hostname = strings.TrimSpace(hostname)
	// Reject obvious user mistakes early. We don't try to validate the
	// token format itself — cloudflared will tell us soon enough if it's
	// wrong, and the format isn't publicly specified anyway.
	if hostname != "" {
		if strings.Contains(hostname, "://") || strings.Contains(hostname, "/") {
			return errors.New("hostname must be a bare host, e.g. prism.example.com (no scheme, no path)")
		}
	}
	if (token == "") != (hostname == "") {
		return errors.New("token and hostname must both be set or both empty")
	}
	// Both fields go through one atomic file rewrite. Two separate
	// writes would expose a window where the file has token without
	// hostname (or vice versa) if the second write fails — exactly
	// the invariant we just validated.
	return writeNestedStringFieldsToYAML(
		filepath.Join(a.dataDir, "prism.yaml"),
		"cloudflare",
		map[string]string{
			"tunnel_token":    token,
			"tunnel_hostname": hostname,
		},
	)
}

// writeStringFieldToYAML rewrites prism.yaml so that the named top-level
// scalar key equals value. Existing comments, ordering and untouched
// fields are preserved. Empty value removes the key entirely. The write
// is atomic via tmp+rename, so a crash mid-write can't corrupt the file.
// Concurrent callers are serialised by prismYAMLMu (see top of file).
//
// The value is always emitted as a quoted YAML string (`key: "value"`)
// so that scalars like "127.0.0.1:39527" or hostnames containing dots
// can never be misparsed as integers / booleans / unintended types.
func writeStringFieldToYAML(path, key, value string) error {
	prismYAMLMu.Lock()
	defer prismYAMLMu.Unlock()
	body, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("read %s: %w", path, err)
		}
		body = nil
	}

	var newLine string
	if value != "" {
		newLine = fmt.Sprintf("%s: %q", key, value)
	}

	prefix := key + ":"
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	var out bytes.Buffer
	replaced := false
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimLeft(line, " \t")
		if trimmed == line && strings.HasPrefix(trimmed, prefix) {
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

// readNestedStringFromYAML reads a two-level nested key (parent: child:
// value) without bringing in a full YAML parser. We deliberately keep
// this minimal: only one level of nesting, only string scalars, only
// space-indented (no tabs). Good enough for prism.yaml's hand-curated
// shape; nothing else writes there.
//
// Returns ("", nil) when the file or the key is missing — the caller
// can't tell those apart, but neither needs to.
func readNestedStringFromYAML(path, parent, child string) (string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	parentPrefix := parent + ":"
	childPrefix := child + ":"
	inParent := false
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimLeft(line, " \t")
		// Blank lines don't change scope — YAML allows them inside
		// block mappings. Without this guard the empty-string case
		// would satisfy `trimmed == line`, flip inParent off, and
		// any sibling key below the blank line would be invisible.
		if trimmed == "" {
			continue
		}
		// Top-level (no indentation): tracks whether we're currently
		// inside the parent block.
		if trimmed == line {
			inParent = strings.HasPrefix(trimmed, parentPrefix)
			continue
		}
		if !inParent {
			continue
		}
		if !strings.HasPrefix(trimmed, childPrefix) {
			continue
		}
		val := strings.TrimPrefix(trimmed, childPrefix)
		return unquoteYAMLScalar(val), nil
	}
	return "", scanner.Err()
}

// writeNestedStringFieldToYAML upserts a single two-level nested key.
// Wraps the multi-field helper so callers that only touch one field
// don't have to construct a map. See writeNestedStringFieldsToYAML.
func writeNestedStringFieldToYAML(path, parent, child, value string) error {
	return writeNestedStringFieldsToYAML(path, parent, map[string]string{child: value})
}

// writeNestedStringFieldsToYAML upserts one or more two-level nested
// keys (`parent:\n  child1: v1\n  child2: v2`) in a single atomic
// file write. It preserves the rest of the file and respects the
// pre-existing indentation of the parent block — if the user's other
// children use 4 spaces, ours will too. Defaults to 2 spaces when the
// block is empty / missing.
//
// Empty value for a child removes just that child line; siblings stay.
// Empty value for every child is a no-op for the parent block (we
// don't try to delete the parent itself).
//
// Atomicity is critical: SetCloudflareTunnel needs token + hostname
// to land together so the "both set or both empty" invariant can't
// be violated by a crash between two separate file rewrites.
//
// Concurrent callers are serialised by prismYAMLMu (see top of file).
func writeNestedStringFieldsToYAML(path, parent string, children map[string]string) error {
	if len(children) == 0 {
		return nil
	}
	prismYAMLMu.Lock()
	defer prismYAMLMu.Unlock()
	body, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("read %s: %w", path, err)
		}
		body = nil
	}
	parentPrefix := parent + ":"

	// Stable iteration order so freshly-created parent blocks are
	// deterministic across runs / test invocations.
	keys := make([]string, 0, len(children))
	for k := range children {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	replaced := make(map[string]bool, len(children))

	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	var (
		out            bytes.Buffer
		inParent       bool
		parentExists   bool
		detectedIndent string // indentation seen on existing siblings
	)
	defaultIndent := "  "

	// Flush children that haven't matched an existing line yet. Called
	// either at end-of-parent (top-level transition) or end-of-file.
	flushPendingBeforeLeavingParent := func() {
		indent := detectedIndent
		if indent == "" {
			indent = defaultIndent
		}
		for _, k := range keys {
			if replaced[k] {
				continue
			}
			v := children[k]
			if v == "" {
				// Caller wants this child removed; no existing
				// line was found, so nothing to do.
				replaced[k] = true
				continue
			}
			out.WriteString(indent)
			out.WriteString(fmt.Sprintf("%s: %q", k, v))
			out.WriteByte('\n')
			replaced[k] = true
		}
	}

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimLeft(line, " \t")
		// Blank lines are allowed inside YAML block mappings without
		// terminating the parent. Echo them verbatim and KEEP inParent
		// as-is — flipping it here would prematurely flush the new
		// children above any siblings that come after the blank line,
		// and real children below the blank line would be missed.
		if trimmed == "" {
			out.WriteString(line)
			out.WriteByte('\n')
			continue
		}
		isTopLevel := trimmed == line
		if isTopLevel {
			// Leaving the previous parent — flush pending children
			// before the new top-level block begins.
			if inParent {
				flushPendingBeforeLeavingParent()
			}
			inParent = strings.HasPrefix(trimmed, parentPrefix)
			if inParent {
				parentExists = true
				detectedIndent = ""
			}
			out.WriteString(line)
			out.WriteByte('\n')
			continue
		}
		// Non-top-level inside parent: capture indentation and check
		// whether this line is one of the children we're upserting.
		if inParent {
			indent := line[:len(line)-len(trimmed)]
			if detectedIndent == "" {
				detectedIndent = indent
			}
			matchedKey := ""
			for _, k := range keys {
				if strings.HasPrefix(trimmed, k+":") {
					matchedKey = k
					break
				}
			}
			if matchedKey != "" {
				v := children[matchedKey]
				if v != "" {
					out.WriteString(indent)
					out.WriteString(fmt.Sprintf("%s: %q", matchedKey, v))
					out.WriteByte('\n')
				}
				replaced[matchedKey] = true
				continue
			}
		}
		out.WriteString(line)
		out.WriteByte('\n')
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan %s: %w", path, err)
	}

	// File ended while still inside the parent block — flush pending
	// children onto the tail.
	if inParent {
		flushPendingBeforeLeavingParent()
	}

	// Parent didn't exist at all and we have at least one non-empty
	// value to write — append a fresh parent block at the bottom.
	if !parentExists {
		anyToAppend := false
		for _, v := range children {
			if v != "" {
				anyToAppend = true
				break
			}
		}
		if anyToAppend {
			if out.Len() > 0 && out.Bytes()[out.Len()-1] != '\n' {
				out.WriteByte('\n')
			}
			out.WriteString(parentPrefix)
			out.WriteByte('\n')
			for _, k := range keys {
				v := children[k]
				if v == "" {
					continue
				}
				out.WriteString(defaultIndent)
				out.WriteString(fmt.Sprintf("%s: %q", k, v))
				out.WriteByte('\n')
			}
		}
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
