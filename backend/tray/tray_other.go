//go:build !darwin

package tray

// Start is a no-op on non-macOS targets. Prism currently ships only for
// macOS; Linux / Windows tray support will require a different backend
// (libappindicator / Shell_NotifyIcon respectively) and is deferred.
func Start(iconPath string) {}

// SetTitle is a no-op outside macOS.
func SetTitle(title string) {}

// Add returns nil so call sites that chain AddSubItem on the result won't panic.
func Add(title string, enabled bool, handler func()) *MenuItem { return nil }

// AddSeparator is a no-op outside macOS.
func AddSeparator() {}

// AddSubItem is a no-op outside macOS.
func (p *MenuItem) AddSubItem(title string, enabled bool, handler func()) *MenuItem {
	return nil
}

// SetTitle on the item is a no-op outside macOS.
func (m *MenuItem) SetTitle(title string) {}

// SetEnabled on the item is a no-op outside macOS.
func (m *MenuItem) SetEnabled(enabled bool) {}

// SetDockVisible is a no-op outside macOS.
func SetDockVisible(visible bool) {}
