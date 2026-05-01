//go:build darwin

package tray

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#include <stdlib.h>

void prism_tray_init(void);
void prism_tray_set_icon(const char* path);
void prism_tray_set_title(const char* title);
int  prism_tray_add_item(const char* title, int enabled);
int  prism_tray_add_sub_item(int parentId, const char* title, int enabled);
void prism_tray_add_separator(void);
void prism_tray_update_item_title(int id, const char* title);
void prism_tray_update_item_enabled(int id, int enabled);
void prism_tray_set_dock_visible(int visible);

extern void prismTrayItemClicked(int id);
*/
import "C"
import (
	"runtime"
	"sync"
	"unsafe"
)

var (
	mu       sync.Mutex
	handlers = map[int]func(){}
	byID     = map[int]*MenuItem{}
	started  bool
)

// Start creates the NSStatusItem. Must be called on the main thread (the Go
// side of `wails.Run` invokes this before handing control over to the Cocoa
// run loop, which is the first and only safe window).
//
// iconPath points to a PNG on disk. The icon is sized down to the menu bar
// thickness automatically; pass a 32x32 or 22x22 image.
func Start(iconPath string) {
	mu.Lock()
	defer mu.Unlock()
	if started {
		return
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	C.prism_tray_init()

	if iconPath != "" {
		cPath := C.CString(iconPath)
		C.prism_tray_set_icon(cPath)
		C.free(unsafe.Pointer(cPath))
	}
	started = true
}

// SetTitle sets the text label shown next to the icon (or standalone if no
// icon was configured). Useful for exposing status like "● Core".
func SetTitle(title string) {
	if !started {
		return
	}
	cTitle := C.CString(title)
	C.prism_tray_set_title(cTitle)
	C.free(unsafe.Pointer(cTitle))
}

// Add appends a clickable item to the root menu. Pass enabled=false for a
// grayed-out informational row (the handler is still ignored in that case).
func Add(title string, enabled bool, handler func()) *MenuItem {
	mu.Lock()
	defer mu.Unlock()
	if !started {
		return nil
	}
	cTitle := C.CString(title)
	defer C.free(unsafe.Pointer(cTitle))

	en := C.int(0)
	if enabled {
		en = 1
	}

	id := int(C.prism_tray_add_item(cTitle, en))
	item := &MenuItem{id: id, title: title, handler: handler}
	handlers[id] = handler
	byID[id] = item
	return item
}

// AddSeparator inserts a divider line into the root menu.
func AddSeparator() {
	if !started {
		return
	}
	C.prism_tray_add_separator()
}

// AddSubItem attaches a child item to the given parent. Useful for nested
// actions but Prism currently only uses the flat root menu.
func (p *MenuItem) AddSubItem(title string, enabled bool, handler func()) *MenuItem {
	mu.Lock()
	defer mu.Unlock()
	if p == nil {
		return nil
	}
	cTitle := C.CString(title)
	defer C.free(unsafe.Pointer(cTitle))

	en := C.int(0)
	if enabled {
		en = 1
	}

	id := int(C.prism_tray_add_sub_item(C.int(p.id), cTitle, en))
	item := &MenuItem{id: id, title: title, handler: handler}
	handlers[id] = handler
	byID[id] = item
	p.subItems = append(p.subItems, item)
	return item
}

// SetTitle updates an existing menu item's label (e.g. live status line).
func (m *MenuItem) SetTitle(title string) {
	if m == nil {
		return
	}
	cTitle := C.CString(title)
	defer C.free(unsafe.Pointer(cTitle))
	C.prism_tray_update_item_title(C.int(m.id), cTitle)
	m.title = title
}

// SetEnabled toggles whether the menu item is clickable. Useful for e.g.
// disabling "Copy URL" when no tunnel URL has been published yet.
func (m *MenuItem) SetEnabled(enabled bool) {
	if m == nil {
		return
	}
	en := C.int(0)
	if enabled {
		en = 1
	}
	C.prism_tray_update_item_enabled(C.int(m.id), en)
}

// SetDockVisible controls whether the application shows up in the Dock.
// When false, the process switches to NSApplicationActivationPolicyAccessory:
// the window is gone from the Dock but the menu bar item keeps running.
// When true, the Dock icon returns (used right before showing the window
// again so command-tab / red-dot-close work normally).
func SetDockVisible(visible bool) {
	v := C.int(0)
	if visible {
		v = 1
	}
	C.prism_tray_set_dock_visible(v)
}

//export prismTrayItemClicked
func prismTrayItemClicked(id C.int) {
	mu.Lock()
	h := handlers[int(id)]
	mu.Unlock()
	if h != nil {
		go h()
	}
}
