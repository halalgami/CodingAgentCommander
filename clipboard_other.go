//go:build !darwin

package main

// Only macOS reads the pasteboard natively; nil makes PasteClipboardFiles
// refuse and the frontend fall back to the webview's own paste bytes.
var clipboardRead func(destBase string) (clipContents, error)
