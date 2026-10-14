package main

import (
	"context"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// openFilesDialog is the native multi-file open dialog; a variable so tests
// can stand in for it.
var openFilesDialog = func(ctx context.Context) ([]string, error) {
	return wruntime.OpenMultipleFilesDialog(ctx, wruntime.OpenDialogOptions{Title: "Attach files"})
}

// PickAttachFiles lets the user choose files to insert (decision D9). This is
// the path for keyboard users and for platforms without drag and drop. Cancel
// returns an empty list, never nil (nil marshals as null).
func (a *App) PickAttachFiles() ([]string, error) {
	paths, err := openFilesDialog(a.ctx)
	if err != nil {
		return nil, err
	}
	if paths == nil {
		paths = []string{}
	}
	return paths, nil
}
