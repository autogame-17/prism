package main

import (
	"context"
	"sync/atomic"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"prism/backend/bindings"
	"prism/backend/embed"
	"prism/backend/tray"
	"prism/backend/tunnel"
)

// App is the root Wails application struct. It owns the embedded core and
// forwards lifecycle events to domain bindings.
type App struct {
	ctx context.Context

	// quitting flips to true the moment Quit() is invoked (e.g. from the
	// status-bar "Quit Prism" item). BeforeClose checks this flag so that
	// an explicit quit is allowed to proceed, while red-dot / Cmd+W on
	// the window still hides Prism into the menu bar.
	quitting atomic.Bool

	boot     *embed.BootResult
	tun      *tunnel.Cloudflared
	system   *bindings.SystemAPI
	tunnel   *bindings.TunnelAPI
	channels *bindings.ChannelsAPI
	tokens   *bindings.TokensAPI
	logs     *bindings.LogsAPI
	traces   *bindings.TracesAPI
	settings *bindings.SettingsAPI
}

// NewApp creates a new App application struct
func NewApp() *App { return &App{} }

func (a *App) Show() {
	if a.ctx == nil {
		return
	}
	// Restore the Dock icon first; otherwise NSWindow.makeKeyAndOrderFront
	// won't give the accessory-mode process keyboard focus, and the window
	// shows up behind everything.
	tray.SetDockVisible(true)
	runtime.WindowShow(a.ctx)
	runtime.WindowUnminimise(a.ctx)
}

// Hide is called when the user closes the window via the red dot or
// Cmd+W. We keep the backend process alive (one-hub core, Cloudflare
// tunnel, trace store) but remove the Dock icon so Prism looks like a
// true menu bar app.
func (a *App) Hide() {
	if a.ctx == nil {
		return
	}
	runtime.WindowHide(a.ctx)
	tray.SetDockVisible(false)
}

// Navigate asks the frontend to switch to a hash route (e.g. "/logs").
// Always calls Show() first so the window is visible and frontmost.
func (a *App) Navigate(path string) {
	if a.ctx == nil {
		return
	}
	a.Show()
	runtime.EventsEmit(a.ctx, "prism:navigate", path)
}

// Quit terminates the Wails application, stopping the core, tunnel and
// menu bar item. Used by the "Quit Prism" row in the status bar menu.
//
// Important: BeforeClose normally returns true (prevent close) so the red
// dot / Cmd+W only hide the window. We flip quitting=true here so the
// upcoming BeforeClose call lets the close proceed.
func (a *App) Quit() {
	if a.ctx == nil {
		return
	}
	a.quitting.Store(true)
	runtime.Quit(a.ctx)
}

// Boot runs before Wails main loop; boots one-hub core so bindings can use
// model.* etc. We return so that main() can install bindings that reference
// the core state.
//
// ListenAddr is intentionally not set here so embed.Boot picks it up from
// prism.yaml (`listen_addr`), defaulting to 127.0.0.1:39527. Keeping the
// port stable across restarts is what lets external clients (Cursor / IDE
// extensions) hold a single URL.
func (a *App) Boot(version string) error {
	res, err := embed.Boot(embed.BootOptions{
		AppName: "Prism",
	})
	if err != nil {
		return err
	}
	a.boot = res
	a.tun = &tunnel.Cloudflared{
		Binary:         cloudflaredBinaryPath(),
		LocalPort:      httpPort(res.HTTPAddr),
		TunnelToken:    res.TunnelToken,
		PublicHostname: res.TunnelHostname,
	}
	a.system = bindings.NewSystemAPI(version, res)
	a.tunnel = bindings.NewTunnelAPI(a.tun)
	a.channels = bindings.NewChannelsAPI()
	a.tokens = bindings.NewTokensAPI()
	a.logs = bindings.NewLogsAPI()
	a.traces = bindings.NewTracesAPI()
	a.settings = bindings.NewSettingsAPI(res.DataDir, res.LogDir)
	a.settings.SetActualListenAddr(res.HTTPAddr)
	return nil
}

// Startup is called once the Wails runtime is ready.
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	a.system.AttachContext(ctx)
	a.tunnel.Startup(ctx)
	a.logs.SetContext(ctx)
	a.logs.StartSystemLogStream()
	a.traces.SetContext(ctx)
	a.settings.SetContext(ctx)
}

// BeforeClose is wired to Wails' OnBeforeClose hook in main.go. Returning
// `true` tells Wails to abort the close and leave the process running; we
// then manually hide the window and drop the Dock icon so Prism continues
// as a pure menu bar app.
//
// When Quit() has been invoked (status-bar "Quit Prism"), the quitting flag
// is set; in that case we do NOT prevent the close, so Wails proceeds with
// shutdown and OnShutdown can clean up tunnel + DB.
func (a *App) BeforeClose(ctx context.Context) (prevent bool) {
	if a.quitting.Load() {
		return false
	}
	a.Hide()
	return true
}

// Shutdown is invoked by Wails on application exit.
func (a *App) Shutdown(ctx context.Context) {
	if a.logs != nil {
		a.logs.Shutdown()
	}
	if a.tun != nil {
		_ = a.tun.Stop()
	}
	embed.Shutdown(a.boot)
}

// Bindings returns the slice of binding objects Wails should expose to JS.
func (a *App) Bindings() []interface{} {
	return []interface{}{
		a.system,
		a.tunnel,
		a.channels,
		a.tokens,
		a.logs,
		a.traces,
		a.settings,
	}
}

// Greet is a placeholder kept for the default React template so the dev
// server can exercise a binding immediately.
func (a *App) Greet(name string) string {
	return "Hello " + name + ", Prism is booting."
}
