// Package tray implements a macOS status bar (menu bar) item for Prism.
//
// Why this is hand-rolled instead of using a 3rd party systray library:
//
//   - github.com/getlantern/systray ships its own Objective-C `AppDelegate`,
//     which collides with Wails v2's AppDelegate at link time (duplicate
//     symbols).
//   - github.com/energye/systray fixes the symbol clash but starts the
//     NSApplication run loop itself, which fights with Wails for the main
//     thread on macOS 10.15+. The icon silently never appears.
//
// The implementation below is based on the pattern shared by @alexec in
// wailsapp/wails discussion #4514. It only creates an NSStatusItem and
// attaches a plain NSMenu to it; every UI call is dispatched onto the Cocoa
// main queue via `dispatch_async`, which cooperates with Wails' own run loop.
//
// This file is platform-agnostic. The real work lives in tray_darwin.go /
// tray_darwin.m (macOS) and tray_other.go (stub for Linux / Windows, where
// Prism currently targets macOS only).
package tray

// MenuItem is an opaque handle to an item added via Add or AddSubItem.
type MenuItem struct {
	id       int
	title    string
	handler  func()
	subItems []*MenuItem
}

// Handler returns the click handler for the item.
func (m *MenuItem) Handler() func() { return m.handler }
