package main

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// ListProjectDocs lists every document in one project, newest first.
func (a *App) ListProjectDocs(root string) (DocListing, error) {
	real, err := docRoot(root)
	if err != nil {
		return DocListing{}, err
	}
	names, truncated, err := gitListDocs(real)
	if err != nil {
		names, truncated = walkListDocs(real)
	}
	entries := statDocs(real, names)
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].ModTime != entries[j].ModTime {
			return entries[i].ModTime > entries[j].ModTime
		}
		return entries[i].Rel < entries[j].Rel // stable for equal mtimes
	})
	return DocListing{Entries: entries, Root: real, Truncated: truncated}, nil
}

// RenderProjectDoc reads one document and returns finished, sanitized HTML.
// The frontend parses nothing (spec R2.2).
func (a *App) RenderProjectDoc(root, rel string) (DocRender, error) {
	src, err := readDocSource(root, rel)
	if err != nil {
		return DocRender{}, err
	}
	return renderDoc(rel, src), nil
}

// ListSessionDocs lists the documents of the folder one session is running in,
// flagging those that changed since it launched.
//
// Both facts are read from the registry rather than passed in: a windowID is
// all the frontend needs to know, and a launch timestamp never has to cross
// the binding. This lives here rather than as a SessionStats field precisely
// because app.go has an override copy the export overlays wholesale — a new
// field there would need mirroring by hand, and override_parity_test.go's list
// is curated, so nothing would catch the miss.
//
// The returned DocListing.Root is inherited from the ListProjectDocs call
// below, not set separately — it is the session's cwd, resolved.
//
// LaunchedAt is not always when Claude started. reconcile (app.go, ~line
// 1568) sets it to when this APP first saw a session recovered from tmux
// after a restart — the tmux session survives an app restart but the
// in-memory registry does not — rather than to when the underlying Claude
// process actually began. So for a session recovered that way, "since this
// session started" is honestly "since this app instance started". That is a
// real, user-visible difference (a long-lived tmux session restarted five
// minutes ago will show everything touched in the last five minutes as
// "since this session"), but it is not fixed here: fixing it needs the
// process's actual start time, which reconcile does not have and this
// binding has no way to recover after the fact.
func (a *App) ListSessionDocs(windowID string) (DocListing, error) {
	if strings.TrimSpace(windowID) == "" {
		return DocListing{}, fmt.Errorf("no session chosen")
	}
	// Copy out under the lock, then touch the filesystem: never hold a.mu
	// across I/O (a keychain read once froze the whole app that way).
	a.mu.Lock()
	rec := a.sessions[windowID]
	var cwd string
	var launched time.Time
	if rec != nil {
		cwd, launched = rec.Cwd, rec.LaunchedAt
	}
	a.mu.Unlock()
	if rec == nil {
		return DocListing{}, fmt.Errorf("that session is no longer running")
	}
	if strings.TrimSpace(cwd) == "" {
		return DocListing{}, fmt.Errorf("that session has no folder recorded")
	}

	listing, err := a.ListProjectDocs(cwd)
	if err != nil {
		return DocListing{}, err
	}
	cut := launched.Unix()
	for i := range listing.Entries {
		listing.Entries[i].SinceStart = listing.Entries[i].ModTime >= cut
	}
	return listing, nil
}

// docOpener is the seam: swapped in tests so a refused path can be proven not
// to spawn anything.
var docOpener = openInDefaultApp

// OpenProjectDoc hands one file to the OS default application. This is the
// universal escape hatch (spec R2.3): it validates the path with the same
// guard, then declines to care what the file IS — which is how PDF, docx,
// xlsx and images are supported without a single untrusted binary parser in
// this process.
//
// Note it does NOT go through readDocSource: the size cap and the binary sniff
// are reasons to send a file HERE, not reasons to refuse it here.
func (a *App) OpenProjectDoc(root, rel string) error {
	path, err := docPath(root, rel)
	if err != nil {
		return err
	}
	return docOpener(path)
}
