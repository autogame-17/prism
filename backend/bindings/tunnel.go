package bindings

import (
	"context"
	"fmt"
	"sync"
	"time"

	"prism/backend/notify"
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

	// lastAnnouncedURL remembers the URL we already announced to the
	// user so the cold-boot URL is silent and only genuine rotations
	// trigger the clipboard / system-notification flow.
	urlMu            sync.Mutex
	lastAnnouncedURL string
}

// NewTunnelAPI constructs the binding with cloudflared already configured.
func NewTunnelAPI(mgr *tunnel.Cloudflared) *TunnelAPI {
	api := &TunnelAPI{mgr: mgr, maxLog: 500}
	mgr.OnURL = func(url string) {
		if api.ctx != nil {
			wruntime.EventsEmit(api.ctx, "tunnel.url", url)
			wruntime.EventsEmit(api.ctx, "tunnel.status", mgr.Snapshot())
		}
		api.maybeAnnounceURL(url)
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

// maybeAnnounceURL reacts to OnURL callbacks from the cloudflared manager.
// Each genuine rotation (i.e. the URL is not empty AND differs from the
// last URL we already announced) is treated as an event the user almost
// certainly cares about: we copy the OpenAI-compatible base URL to the
// clipboard and surface a native notification, so even with the Prism
// window hidden in the menu bar they have everything they need to fix
// their client.
//
// The first URL after a cold boot is announced too (lastAnnouncedURL is
// empty), but ONLY via the in-window toast wired up in App.tsx — the
// in-window watcher debounces cold-boot itself. Here we don't have to
// duplicate that logic because the OS-level notification on first URL
// is actually useful: it confirms cloudflared came up successfully.
func (a *TunnelAPI) maybeAnnounceURL(url string) {
	if url == "" {
		return
	}
	a.urlMu.Lock()
	previous := a.lastAnnouncedURL
	a.lastAnnouncedURL = url
	a.urlMu.Unlock()
	if previous == url {
		return
	}
	baseURL := url + "/v1"
	title := "Prism tunnel URL ready"
	if previous != "" {
		title = "Prism tunnel URL changed"
	}
	// Run the OS-level helpers off the cloudflared log-parser goroutine
	// so that a slow / hung notify-send / xclip / powershell never delays
	// the next tunnel.url emit or backs up cloudflared's stdout buffer.
	go func() {
		notify.Copy(baseURL)
		notify.Show(title, baseURL+" — copied to clipboard")
	}()
}
