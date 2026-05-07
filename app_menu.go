package main

import (
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"context"
)

func getAppMenu(ctx context.Context) *menu.Menu {
	AppMenu := menu.NewMenu()
	FileMenu := AppMenu.AddSubmenu("Prism")
	FileMenu.AddText("About", keys.CmdOrCtrl("i"), func(_ *menu.CallbackData) {
		runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
			Title:   "About Prism",
			Message: "Prism LLM Gateway v0.1.0\nBuilt on one-hub core.",
		})
	})
	FileMenu.AddSeparator()
	FileMenu.AddText("Quit", keys.CmdOrCtrl("q"), func(_ *menu.CallbackData) {
		runtime.Quit(ctx)
	})

	return AppMenu
}
