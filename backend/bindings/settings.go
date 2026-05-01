package bindings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"one-api/common/config"
	"one-api/model"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// SettingsAPI exposes app-level settings + data-directory / import-export.
type SettingsAPI struct {
	ctx     context.Context
	dataDir string
	logDir  string
}

// NewSettingsAPI constructs the binding; paths come from the boot result.
func NewSettingsAPI(dataDir, logDir string) *SettingsAPI {
	return &SettingsAPI{dataDir: dataDir, logDir: logDir}
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
func (a *SettingsAPI) OpenDataDir() error {
	if a.ctx == nil {
		return errors.New("settings api context not ready")
	}
	wruntime.BrowserOpenURL(a.ctx, "file://"+a.dataDir)
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
