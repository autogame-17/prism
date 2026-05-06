// Package notify provides best-effort cross-platform helpers for
// surfacing information to the user even when Prism's window is hidden:
//   - Copy(text)       — write a string to the system clipboard
//   - Show(title, msg) — raise a native notification (banner / toast)
//
// Failures are intentionally swallowed: these are courtesy hints, not
// hard requirements. We never want a flaky notification daemon to break
// the actual tunnel-rotation flow that called us.
//
// Per-platform strategy:
//
//	macOS   — pbcopy / osascript display notification
//	Windows — clip.exe / PowerShell System.Windows.Forms NotifyIcon
//	Linux   — wl-copy → xclip → xsel (first that works) / notify-send
package notify

import (
	"os/exec"
	"runtime"
	"strings"
)

// Copy writes text to the OS clipboard. Returns true if the underlying
// helper exited cleanly. We never error out — Copy is purely a UX nicety
// layered on top of the in-window toast.
func Copy(text string) bool {
	switch runtime.GOOS {
	case "darwin":
		return runWithStdin(text, "pbcopy")
	case "windows":
		// `clip.exe` is shipped with every Windows version since Vista.
		return runWithStdin(text, "clip")
	case "linux":
		// Wayland-first probing: wl-copy is preferred on modern desktops
		// (GNOME 40+ / KDE Plasma 5.24+) and silently no-ops on X11. Then
		// xclip (most common) → xsel (older / minimal installs). The
		// first helper that exits 0 wins; missing binaries return ENOENT
		// from exec which we treat as "try the next one".
		for _, attempt := range [][]string{
			{"wl-copy"},
			{"xclip", "-selection", "clipboard"},
			{"xsel", "--clipboard", "--input"},
		} {
			if runWithStdin(text, attempt[0], attempt[1:]...) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// Show raises a native notification. Returns true if the helper exited
// cleanly. We never escalate to a modal dialog because that would steal
// focus, which is exactly what we're trying to avoid by going through
// the system-notification route.
func Show(title, body string) bool {
	switch runtime.GOOS {
	case "darwin":
		return showDarwin(title, body)
	case "windows":
		return showWindows(title, body)
	case "linux":
		return showLinux(title, body)
	default:
		return false
	}
}

// runWithStdin pipes text into a helper command's stdin and returns true
// if it exited cleanly. Any error (binary missing, non-zero exit, helper
// hung) collapses to false so callers can fall through to the next
// fallback.
func runWithStdin(text, name string, args ...string) bool {
	cmd := exec.Command(name, args...)
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run() == nil
}

// applescriptEscape escapes a string so it can be safely embedded
// inside an AppleScript double-quoted literal. AppleScript treats
// backslash as the escape character (\n, \t, \r, \\, \") inside
// double-quoted strings, so backslashes MUST be escaped first —
// otherwise a later "\"" we write to escape a real quote would be
// mangled by an earlier replacement of "\" → "\\". A literal
// backslash followed by 'n' in user input would also otherwise
// render as a newline in the notification.
func applescriptEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

func showDarwin(title, body string) bool {
	script := `display notification "` + applescriptEscape(body) +
		`" with title "` + applescriptEscape(title) + `"`
	return exec.Command("osascript", "-e", script).Run() == nil
}

// showWindows raises a balloon tip via System.Windows.Forms.NotifyIcon.
// On Windows 10 / 11 the OS auto-promotes balloon tips to native toasts
// in Action Center, which is exactly what we want; on Windows 7 / 8.1
// the user still gets a tray balloon. No external dependency required:
// .NET Framework ships with all supported Windows versions.
//
// Why not Windows.UI.Notifications.ToastNotificationManager? It needs an
// AppUserModelID registered with the OS, which a Wails app would have to
// set up via shell shortcut hijinks. NotifyIcon avoids that entirely.
func showWindows(title, body string) bool {
	// PowerShell single-quoted strings escape ' by doubling it.
	psEscape := func(s string) string { return strings.ReplaceAll(s, "'", "''") }
	script := `Add-Type -AssemblyName System.Windows.Forms;` +
		`Add-Type -AssemblyName System.Drawing;` +
		`$n = New-Object System.Windows.Forms.NotifyIcon;` +
		`$n.Icon = [System.Drawing.SystemIcons]::Information;` +
		`$n.Visible = $true;` +
		`$n.BalloonTipTitle = '` + psEscape(title) + `';` +
		`$n.BalloonTipText = '` + psEscape(body) + `';` +
		`$n.ShowBalloonTip(8000);` +
		// Sleep just long enough for the OS to pick up the balloon
		// before we dispose; otherwise it never renders.
		`Start-Sleep -Milliseconds 9000;` +
		`$n.Dispose()`
	// -NoProfile keeps PowerShell from sourcing a slow user profile;
	// -WindowStyle Hidden prevents a console flash. -STA is required
	// for any Windows.Forms / WPF interop.
	cmd := exec.Command("powershell",
		"-NoProfile",
		"-NonInteractive",
		"-WindowStyle", "Hidden",
		"-STA",
		"-Command", script,
	)
	if err := cmd.Start(); err != nil {
		return false
	}
	// PowerShell sleeps ~9s before disposing the icon, so we mustn't
	// block the OnURL callback for that long. Reap the child in the
	// background so its process struct is released eventually.
	go func() { _ = cmd.Wait() }()
	return true
}

// showLinux uses notify-send from libnotify, the de-facto desktop
// notification tool present on virtually every modern Linux desktop
// (GNOME, KDE, XFCE, sway, …). If the user lacks libnotify we silently
// skip the notification — Copy() probably also failed in that case, but
// the in-window toast still works when Prism's window is open.
func showLinux(title, body string) bool {
	cmd := exec.Command("notify-send",
		"--app-name", "Prism",
		"--urgency", "normal",
		"--expire-time", "8000",
		title,
		body,
	)
	return cmd.Run() == nil
}
