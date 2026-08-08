package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed all:frontend/dist
var assets embed.FS

// Injected at build time via -ldflags (see Makefile). Defaults are dev values.
var (
	appVersion   = "dev"
	appCommit    = ""
	appBuildDate = ""
)

func main() {
	ensureLoginPATH() // must run before anything shells out (tmux/claude/litellm)

	// Create an instance of the app structure
	app := NewApp()

	// Custom app menu: a native "About Commander" that opens the in-app panel
	// (via the menu:about event), plus EditMenu so ⌘C/⌘V/⌘X/⌘A keep working
	// (a custom menu replaces Wails' default, which otherwise carries them).
	appMenu := menu.NewMenu()
	appMenu.Append(menu.SubMenu("Commander", menu.NewMenuFromItems(
		menu.Text("About Commander", nil, func(_ *menu.CallbackData) {
			wruntime.EventsEmit(app.ctx, "menu:about")
		}),
		menu.Separator(),
		menu.Text("Quit Commander", keys.CmdOrCtrl("q"), func(_ *menu.CallbackData) {
			wruntime.Quit(app.ctx)
		}),
	)))
	appMenu.Append(menu.EditMenu())

	// Create application with options
	err := wails.Run(&options.App{
		Title:  "Commander",
		Width:  1200,
		Height: 800,
		Menu:   appMenu,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 20, G: 17, B: 13, A: 1}, // matches --surface-0
		Mac: &mac.Options{
			TitleBar: mac.TitleBarHiddenInset(),
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
