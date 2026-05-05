package bindings

import (
	"context"
	"fmt"
	"sync"
	"time"

	"prism/backend/tunnel"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// TunnelAPI manages the Cloudflare tunnel lifecycle on behalf of the UI.
type TunnelAPI struct {
	ctx context.Context
	mgr *tunnel.Cloudflared

	mu     sync.Mutex
	logs   []string
	maxLog int
}

// NewTunnelAPI constructs the binding with cloudflared already configured.
func NewTunnelAPI(mgr *tunnel.Cloudflared) *TunnelAPI {
	api := &TunnelAPI{mgr: mgr, maxLog: 500}
	mgr.OnURL = func(url string) {
		if api.ctx != nil {
			wruntime.EventsEmit(api.ctx, "tunnel.url", url)
			wruntime.EventsEmit(api.ctx, "tunnel.status", mgr.Snapshot())
		}
	}
	mgr.LogWriter = logSink{api: api}
	return api
}

// Startup is called by Wails with the runtime context. Guards against a nil
// ctx which happens if the frontend accidentally invokes this binding.
func (a *TunnelAPI) Startup(ctx context.Context) {
	if ctx == nil {
		return
	}
	a.ctx = ctx
}

// Status returns the current tunnel snapshot.
func (a *TunnelAPI) Status() tunnel.Snapshot { return a.mgr.Snapshot() }

// Start launches the tunnel and waits up to 30s for a URL. Errors bubble up.
func (a *TunnelAPI) Start() (string, error) {
	url, err := a.mgr.StartAndWaitURL(30 * time.Second)
	if a.ctx != nil {
		wruntime.EventsEmit(a.ctx, "tunnel.status", a.mgr.Snapshot())
	}
	if err != nil {
		return "", err
	}
	return url, nil
}

// Stop terminates the tunnel.
func (a *TunnelAPI) Stop() error {
	err := a.mgr.Stop()
	if a.ctx != nil {
		wruntime.EventsEmit(a.ctx, "tunnel.status", a.mgr.Snapshot())
	}
	return err
}

// Rotate restarts the tunnel and returns the new URL (if successful).
func (a *TunnelAPI) Rotate() (string, error) {
	url, err := a.mgr.Rotate(30 * time.Second)
	if a.ctx != nil {
		wruntime.EventsEmit(a.ctx, "tunnel.status", a.mgr.Snapshot())
	}
	return url, err
}

// Logs returns the last N log lines the tunnel process emitted.
func (a *TunnelAPI) Logs() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]string, len(a.logs))
	copy(out, a.logs)
	return out
}

// pushLog is invoked by the cloudflared log pipe.
func (a *TunnelAPI) pushLog(line string) {
	a.mu.Lock()
	a.logs = append(a.logs, line)
	if len(a.logs) > a.maxLog {
		a.logs = a.logs[len(a.logs)-a.maxLog:]
	}
	a.mu.Unlock()
	if a.ctx != nil {
		wruntime.EventsEmit(a.ctx, "tunnel.log", line)
	}
}

type logSink struct{ api *TunnelAPI }

func (s logSink) Write(p []byte) (int, error) {
	s.api.pushLog(string(p))
	return len(p), nil
}

// BaseURL returns the full API base URL (including /v1 suffix) if the tunnel is up.
func (a *TunnelAPI) BaseURL() string {
	snap := a.mgr.Snapshot()
	if snap.URL == "" {
		return ""
	}
	return fmt.Sprintf("%s/v1", snap.URL)
}
