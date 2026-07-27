//go:build windows

package main

import (
	"sync"

	toast "git.sr.ht/~jackmordaunt/go-toast/v2"
)

// nativeNotifier posts Windows toast notifications into the Action Center so a
// finished-session banner carries Commander's own app identity (matching the
// macOS build's app-bundle notifications). It mirrors the darwin nativeNotifier
// type so app.go's construction (`nativeNotifier{}`) is portable.
type nativeNotifier struct{}

// appDataOnce registers Commander's AppID + GUID in the registry a single time.
// SetAppData is global state, so it need only run once per process; a failure is
// non-fatal — Push still shows a banner, just without the custom app name/icon.
var appDataOnce sync.Once

// commanderToastGUID is a stable, Commander-specific GUID so repeated runs update
// the same Action Center registration rather than spawning duplicates.
const commanderToastGUID = "{6F3B9A2C-1E4D-4C7A-9B2E-7C1D5A8F3E10}"

func (nativeNotifier) Notify(title, body string) error {
	appDataOnce.Do(func() {
		_ = toast.SetAppData(toast.AppData{
			AppID: "Commander",
			GUID:  commanderToastGUID,
		})
	})
	n := toast.Notification{
		AppID: "Commander",
		Title: title,
		Body:  body,
	}
	return n.Push()
}
