package main

import (
	"encoding/json"
	"errors"
	"fmt"
)

// User-visible refusals for pasting Finder-copied files (user decision U5).
var (
	errClipboardUnsupported = errors.New("Pasting files isn't supported here — drag them onto the session instead")
	errClipboardNoFiles     = errors.New("The clipboard has no files")
)

// clipContents is what ONE native pasteboard read found: real file paths
// (preferred, a Finder copy) or, when there are none, an image written to disk
// (PNG preferred, else TIFF — what ⌘⇧⌃4 and Photos' Copy leave behind).
type clipContents struct {
	Files  []string `json:"files"`
	Image  string   `json:"image"`
	TooBig int64    `json:"tooBig"` // bytes of an image the reader refused to write
}

// parseClipboard decodes the JSON object the pasteboard reader prints. Anything
// that is not that object is an error, never a partial result.
func parseClipboard(out []byte) (clipContents, error) {
	var c clipContents
	if err := json.Unmarshal(out, &c); err != nil {
		return clipContents{}, fmt.Errorf("could not read the clipboard: %w", err)
	}
	return c, nil
}

// PasteClipboardFiles inserts what the system clipboard holds: files ⌘C copied
// in Finder, or else a copied image. The webview withholds real paths and hands
// over only the formats it knows, so both are read natively (clipboardRead),
// which also makes ⌘V agree with Claude Code's own Ctrl+V. The
// paths are real, so each goes through the same validated sink as every other
// insert (registry, permission-prompt guard, Validate, dedupe, one Paste, never
// Enter) rather than the drop fragment logic.
func (a *App) PasteClipboardFiles(windowID string) (AttachResult, error) {
	if clipboardRead == nil {
		return emptyResult(), errClipboardUnsupported
	}
	// An unknown or waiting window is refused BEFORE anything is read or a
	// file is written for it.
	if _, _, err := a.insertTarget(windowID); err != nil {
		return emptyResult(), err
	}
	base, err := a.attachments().NewPasteBase()
	if err != nil {
		return emptyResult(), err
	}
	c, err := clipboardRead(base)
	if err != nil {
		return emptyResult(), err
	}
	if len(c.Files) == 0 && c.Image == "" && c.TooBig > 0 {
		return emptyResult(), fmt.Errorf("the clipboard image is %d MB, which is too large to paste", c.TooBig>>20)
	}
	paths := c.Files
	if len(paths) == 0 && c.Image != "" {
		paths = []string{c.Image}
	}
	if len(paths) == 0 {
		return emptyResult(), errClipboardNoFiles
	}
	if len(paths) > maxBatch {
		return emptyResult(), errTooMany
	}
	return a.insert(windowID, paths)
}
