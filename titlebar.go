package main

import (
	"strings"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// doubleClickAction maps the raw macOS AppleActionOnDoubleClick value to one of
// "zoom", "minimize" or "none". A missing key (empty) or anything unrecognised
// is "zoom", which is the macOS default.
func doubleClickAction(raw string) string {
	switch strings.TrimSpace(raw) {
	case "Minimize":
		return "minimize"
	case "None":
		return "none"
	}
	return "zoom"
}

// TitleBarDoubleClick is called by the frontend when the user double-clicks the
// bare titlebar. The titlebar is a webview drag region (TitleBarHiddenInset), and
// Wails starts a native drag there but never zooms, so this performs what the
// system setting asks for.
func (a *App) TitleBarDoubleClick() {
	switch doubleClickAction(readDoubleClickPref()) {
	case "minimize":
		wruntime.WindowMinimise(a.ctx)
	case "none":
	default:
		wruntime.WindowToggleMaximise(a.ctx)
	}
}
