package bindings

import (
	"context"
	"time"

	"prism/backend/embed"

	"one-api/model"
)

// SystemAPI exposes system-level info + actions to the Wails frontend.
type SystemAPI struct {
	ctx     context.Context
	boot    *embed.BootResult
	version string
}

// NewSystemAPI constructs the binding.
func NewSystemAPI(version string, boot *embed.BootResult) *SystemAPI {
	return &SystemAPI{version: version, boot: boot}
}

// AttachContext is invoked by Wails with the runtime context. Kept with a
// non-Startup name so the Wails binding generator doesn't expose it to JS.
// Nil-guarded to protect against any accidental invocation.
func (a *SystemAPI) AttachContext(ctx context.Context) {
	if ctx == nil {
		return
	}
	a.ctx = ctx
}

// SystemInfo holds static + dynamic system details surfaced in UI. All
// timestamps are serialised as unix millis because Wails' TS type generator
// can't handle time.Time.
type SystemInfo struct {
	Version       string `json:"version"`
	CoreCommit    string `json:"coreCommit"`
	DataDir       string `json:"dataDir"`
	LogDir        string `json:"logDir"`
	HTTPAddr      string `json:"httpAddr"`
	StartedAt     int64  `json:"startedAt"`
	TotalChannels int64  `json:"totalChannels"`
	TotalTokens   int64  `json:"totalTokens"`
}

// Info returns a snapshot of the running system. Safe to call before DB
// migration finishes; returns zeros for counts in that case.
func (a *SystemAPI) Info() SystemInfo {
	var channelCount, tokenCount int64
	if model.DB != nil {
		model.DB.Model(&model.Channel{}).Count(&channelCount)
		model.DB.Model(&model.Token{}).Count(&tokenCount)
	}
	info := SystemInfo{
		Version:       a.version,
		CoreCommit:    "one-hub-upstream",
		StartedAt:     startedAt.UnixMilli(),
		TotalChannels: channelCount,
		TotalTokens:   tokenCount,
	}
	if a.boot != nil {
		info.DataDir = a.boot.DataDir
		info.LogDir = a.boot.LogDir
		info.HTTPAddr = a.boot.HTTPAddr
	}
	return info
}

var startedAt = time.Now()

// Ping simple health probe callable from the UI.
func (a *SystemAPI) Ping() string { return "pong" }
