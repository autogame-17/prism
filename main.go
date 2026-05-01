package main

import (
	"context"
	"embed"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"prism/backend/tray"
	"prism/backend/tunnel"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/trayicon-template.png
var trayIconPNG []byte

const appVersion = "0.1.0"

func main() {
	app := NewApp()
	if err := app.Boot(appVersion); err != nil {
		log.Fatalf("prism boot failed: %v", err)
	}

	// The menu bar item must be installed after Wails has wired up
	// NSApplication, otherwise [NSStatusBar systemStatusBar] returns a
	// detached instance that never renders. We therefore register the tray
	// setup as a post-startup callback rather than calling it before
	// wails.Run.
	originalStartup := app.Startup
	wrappedStartup := func(ctx context.Context) {
		originalStartup(ctx)
		setupMenuBar(app)
	}

	err := wails.Run(&options.App{
		Title:            "Prism",
		Width:            1120,
		Height:           760,
		MinWidth:         960,
		MinHeight:        640,
		BackgroundColour: &options.RGBA{R: 15, G: 20, B: 32, A: 1},
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup:        wrappedStartup,
		OnBeforeClose:    app.BeforeClose,
		OnShutdown:       app.Shutdown,
		Bind:             app.Bindings(),
		Mac: &mac.Options{
			TitleBar:             mac.TitleBarHiddenInset(),
			Appearance:           mac.NSAppearanceNameDarkAqua,
			WebviewIsTransparent: true,
			WindowIsTranslucent:  true,
		},
	})
	if err != nil {
		log.Printf("wails exit: %v", err)
	}
}

// setupMenuBar installs the macOS status bar icon + dropdown. No-op on
// other platforms (tray package stubs everything out).
func setupMenuBar(app *App) {
	if runtime.GOOS != "darwin" {
		return
	}

	iconPath := resolveTrayIconPath()
	tray.Start(iconPath)

	// Row 1 — Core status. Informational only (always disabled). Title is
	// refreshed every few seconds with the live HTTP listen address.
	coreItem := tray.Add("Core: starting…", false, nil)

	// Row 2 — Tunnel status + URL (disabled). Title updates when cloudflared
	// reports status/URL. When the URL is present, becomes the primary hint
	// for the "Copy Tunnel URL" action right below it.
	tunnelItem := tray.Add("Tunnel: stopped", false, nil)

	tray.AddSeparator()

	// Row 3 — Start/Stop tunnel toggle. Title flips based on current state.
	var tunnelToggle *tray.MenuItem
	tunnelToggle = tray.Add("Start Tunnel", true, func() {
		snap := app.tun.Snapshot()
		if snap.URL != "" || snap.Status == tunnel.StatusRunning || snap.Status == tunnel.StatusStarting {
			_ = app.tun.Stop()
			return
		}
		if _, err := app.tun.StartAndWaitURL(30 * time.Second); err != nil {
			log.Printf("tray: tunnel start failed: %v", err)
		}
	})

	// Row 4 — Copy Tunnel URL. Disabled until a URL is available.
	copyURLItem := tray.Add("Copy Tunnel URL", false, func() {
		snap := app.tun.Snapshot()
		if snap.URL == "" || app.ctx == nil {
			return
		}
		_ = wruntime.ClipboardSetText(app.ctx, snap.URL)
	})

	// Row 5 — Copy local API URL. Always available once core is up.
	copyLocalItem := tray.Add("Copy Local API URL", false, func() {
		if app.boot == nil || app.boot.HTTPAddr == "" || app.ctx == nil {
			return
		}
		_ = wruntime.ClipboardSetText(app.ctx, "http://"+app.boot.HTTPAddr)
	})

	tray.AddSeparator()
	tray.Add("Show Prism", true, func() { app.Show() })
	tray.AddSeparator()
	tray.Add("Quit Prism", true, func() { app.Quit() })

	go refreshTrayLabels(app, coreItem, tunnelItem, tunnelToggle, copyURLItem, copyLocalItem)
}

// refreshTrayLabels polls the embedded core + tunnel state every few seconds
// and pushes the latest status into the (disabled) info rows. It also flips
// "Start Tunnel" ↔ "Stop Tunnel" and enables/disables the Copy items based
// on whether there is actually something to copy.
func refreshTrayLabels(
	app *App,
	coreItem *tray.MenuItem,
	tunnelItem *tray.MenuItem,
	tunnelToggle *tray.MenuItem,
	copyURLItem *tray.MenuItem,
	copyLocalItem *tray.MenuItem,
) {
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()

	update := func() {
		if app.boot != nil && app.boot.HTTPAddr != "" {
			coreItem.SetTitle(fmt.Sprintf("Core: %s", app.boot.HTTPAddr))
			copyLocalItem.SetEnabled(true)
		} else {
			coreItem.SetTitle("Core: offline")
			copyLocalItem.SetEnabled(false)
		}

		if app.tun == nil {
			tunnelItem.SetTitle("Tunnel: unavailable")
			tunnelToggle.SetEnabled(false)
			copyURLItem.SetEnabled(false)
			return
		}

		snap := app.tun.Snapshot()
		switch snap.Status {
		case tunnel.StatusRunning:
			if snap.URL != "" {
				tunnelItem.SetTitle("Tunnel: " + snap.URL)
			} else {
				tunnelItem.SetTitle("Tunnel: running")
			}
			tunnelToggle.SetTitle("Stop Tunnel")
			copyURLItem.SetEnabled(snap.URL != "")
		case tunnel.StatusStarting:
			tunnelItem.SetTitle("Tunnel: starting…")
			tunnelToggle.SetTitle("Stop Tunnel")
			copyURLItem.SetEnabled(false)
		default:
			tunnelItem.SetTitle("Tunnel: stopped")
			tunnelToggle.SetTitle("Start Tunnel")
			copyURLItem.SetEnabled(false)
		}
	}

	update()
	for range tick.C {
		update()
	}
}

// resolveTrayIconPath exports the embedded tray icon onto disk because
// NSImage.initWithContentsOfFile expects a filesystem path. We write it
// into the user cache directory once per boot.
func resolveTrayIconPath() string {
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	dir := filepath.Join(base, "Prism")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}
	dst := filepath.Join(dir, "trayicon-template.png")
	if err := os.WriteFile(dst, trayIconPNG, 0o644); err != nil {
		return ""
	}
	return dst
}

// cloudflaredBinaryPath returns the filesystem path to the cloudflared
// executable Prism should launch. It first tries the binary embedded via
// go:embed; if none is vendored for the current platform, it falls back to
// PATH lookup, which is useful during local development.
func cloudflaredBinaryPath() string {
	cacheDir, err := cloudflaredCacheDir()
	if err != nil {
		cacheDir = filepath.Join(os.TempDir(), "prism-cloudflared")
	}
	p, err := tunnel.ResolveBinary(cacheDir)
	if err != nil {
		log.Printf("cloudflared resolve: %v", err)
		return ""
	}
	return p
}

func cloudflaredCacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "Prism", "cloudflared")
	return dir, nil
}

// httpPort extracts the port from a host:port string.
func httpPort(addr string) int {
	_, p, err := net.SplitHostPort(addr)
	if err != nil {
		return 0
	}
	port, _ := strconv.Atoi(p)
	return port
}
