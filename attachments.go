package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/halalgami/CodingAgentCommander/internal/attach"
)

const (
	mediaAttachPrefix = "/media/attach/"
	// maxBatch caps one insert (decision D10, user decision U4).
	maxBatch = 20
)

// User-visible refusals; the frontend toasts err verbatim.
var (
	errNoSession      = errors.New("no such session")
	errTooMany        = fmt.Errorf("Attach at most %d files at once", maxBatch)
	errTooManyDropped = fmt.Errorf("Drop at most %d files at once", maxBatch)
	errBrokenDrop     = errors.New("A dropped name contains a line break or was moved — nothing was inserted.")
	errNotFiles       = errors.New("Only files can be dropped onto a session.")
)

// Attachment describes one inserted path for the confirmation. Path is what
// went into the prompt; Thumb is a webview URL — never interchangeable.
type Attachment struct {
	ID    string `json:"id"`
	Path  string `json:"path"`
	Name  string `json:"name"`
	Kind  string `json:"kind"`
	Thumb string `json:"thumb"`
	// OriginalName is set when Path is a converted copy: the confirmation shows
	// what the user chose, not convert-<ts>.jpg.
	OriginalName string `json:"originalName"`
}

// Rejection names a refused candidate and why, verbatim for the UI.
type Rejection struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// AttachResult reports both outcomes of one batch: a paste of three files
// with one bad name inserts two and reports one.
type AttachResult struct {
	Inserted []Attachment `json:"inserted"`
	Rejected []Rejection  `json:"rejected"`
}

func emptyResult() AttachResult {
	return AttachResult{Inserted: []Attachment{}, Rejected: []Rejection{}}
}

func (a *App) attachments() *attach.Store {
	a.attachMu.Lock()
	defer a.attachMu.Unlock()
	if a.attachStore == nil {
		a.attachStore = &attach.Store{Dir: attach.DefaultDir()}
	}
	return a.attachStore
}

// describeAttachment builds the confirmation entry and, for images, publishes
// a thumbnail id. The map stores the symlink-RESOLVED path so
// serveAttachMedia's Lstat guard can refuse a symlink planted after
// registration.
func (a *App) describeAttachment(abs string) Attachment {
	fi, _ := os.Stat(abs)
	kind := attach.KindOf(abs, fi != nil && fi.IsDir())
	sum := sha256.Sum256([]byte(abs))
	att := Attachment{ID: hex.EncodeToString(sum[:12]), Path: abs, Name: attach.DisplayName(abs), Kind: kind}
	if kind == "image" {
		if real, err := filepath.EvalSymlinks(abs); err == nil {
			a.attachMu.Lock()
			if a.attachMedia == nil {
				a.attachMedia = map[string]string{}
			}
			a.attachMedia[att.ID] = real
			a.attachMu.Unlock()
			att.Thumb = mediaAttachPrefix + att.ID
		}
	}
	return att
}

// insertTarget reads what an insert needs to know about a window, under a.mu.
// An id the registry does not know is refused, and so is a session showing a
// permission/question prompt (AwaitingSession): pasted text could land in the
// prompt's answer UI.
func (a *App) insertTarget(windowID string) (name, cwd string, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	r := a.sessions[windowID]
	if r == nil {
		return "", "", errNoSession
	}
	name = r.Name
	if name == "" {
		name = windowID
	}
	if r.AwaitingSession != "" {
		return "", "", fmt.Errorf("%s is waiting for you to answer a prompt — answer it, then try again", name)
	}
	return name, r.Cwd, nil
}

// insert is the one sink every front-end ends in. It validates each path,
// refusing rather than sanitizing, and pastes every accepted path into the
// window's prompt in ONE paste — space separated, one trailing space, never
// Enter. If the paste fails, nothing is reported as inserted: the confirmation
// must never claim text that did not arrive.
func (a *App) insert(windowID string, paths []string) (AttachResult, error) {
	res := emptyResult()
	name, cwd, err := a.insertTarget(windowID)
	if err != nil {
		return res, err
	}
	seen := map[string]bool{}
	var accepted []Attachment
	var parts []string
	for _, p := range paths {
		abs, err := attach.Validate(p)
		if err != nil {
			res.Rejected = append(res.Rejected, Rejection{Name: attach.DisplayName(p), Reason: err.Error()})
			continue
		}
		if seen[abs] {
			continue
		}
		seen[abs] = true
		// One normalize stage for every source (drop, paste, picker, capture):
		// unreadable image formats are converted, oversize images refused or
		// downscaled. The converted copy, not the original, goes in the prompt.
		final, err := a.attachments().Normalize(abs, convertImage)
		if err != nil {
			res.Rejected = append(res.Rejected, Rejection{Name: attach.DisplayName(p), Reason: err.Error()})
			continue
		}
		att := a.describeAttachment(final)
		if final != abs {
			att.OriginalName = attach.DisplayName(abs)
		}
		accepted = append(accepted, att)
		parts = append(parts, attach.PromptText(final, cwd))
	}
	if len(parts) == 0 {
		return res, nil
	}
	// Conversions can take a while (30 s each, up to 20 files): the session may
	// have started waiting for an answer, or been killed, since insertTarget.
	if _, _, err := a.insertTarget(windowID); err != nil {
		return res, err
	}
	if err := a.host.Paste(windowID, strings.Join(parts, " ")+" "); err != nil {
		// Wails delivers only the error when one is returned, so the names
		// refused in this batch go into the message or the user never sees them.
		msg := ""
		if len(res.Rejected) > 0 {
			var rs []string
			for _, r := range res.Rejected {
				rs = append(rs, fmt.Sprintf("%s (%s)", r.Name, r.Reason))
			}
			msg = "; also refused: " + strings.Join(rs, ", ")
		}
		return res, fmt.Errorf("could not insert into %s: %w%s", name, err, msg)
	}
	res.Inserted = accepted
	return res, nil
}

// InsertPaths inserts paths from a paste, a capture or the open dialog. Each
// bad path is reported on its own and the rest still go in.
func (a *App) InsertPaths(windowID string, paths []string) (AttachResult, error) {
	if len(paths) > maxBatch {
		return emptyResult(), errTooMany
	}
	return a.insert(windowID, paths)
}

// InsertDroppedPaths inserts a Wails file drop, which must be accepted or
// refused WHOLE. Wails on darwin joins the dropped paths with "\n" and splits
// them again, so a name containing a newline arrives as several entries — and
// a directory named "x\n" can yield a fragment that is a real absolute path
// (say ~/.ssh/id_rsa). Filtering entry by entry would smuggle that fragment
// in; refusing the drop cannot.
func (a *App) InsertDroppedPaths(windowID string, paths []string) (AttachResult, error) {
	if allEmpty(paths) {
		return emptyResult(), errNotFiles
	}
	if len(paths) > maxBatch {
		return emptyResult(), errTooManyDropped
	}
	if dropLooksSplit(paths) {
		return emptyResult(), errBrokenDrop
	}
	return a.insert(windowID, paths)
}

func allEmpty(paths []string) bool {
	for _, p := range paths {
		if p != "" {
			return false
		}
	}
	return true
}

// dropLooksSplit reports whether a drop shows the marks of a name that Wails
// split on "\n": an empty, relative or missing entry, or a run of adjacent
// entries that re-joins into a path that exists. n <= maxBatch, so the
// re-join scan is at most 190 Lstats.
func dropLooksSplit(paths []string) bool {
	for _, p := range paths {
		_, err := attach.Validate(p)
		if errors.Is(err, attach.ErrNotAbsolute) || errors.Is(err, attach.ErrMissing) {
			return true
		}
	}
	for i := range paths {
		for j := i + 1; j < len(paths); j++ {
			if _, err := os.Lstat(strings.Join(paths[i:j+1], "\n")); err == nil {
				return true
			}
		}
	}
	return false
}

// SaveClipboardImage persists pasted image bytes (base64 from the webview's
// paste event) and returns the saved path, ready for InsertPaths.
func (a *App) SaveClipboardImage(b64, mime string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", errors.New("the clipboard image could not be read")
	}
	return a.attachments().SaveImage(data, mime)
}

// serveAttachMedia serves one confirmation thumbnail by id, with
// servePackMedia's guards: the id is a map key, the extension is re-checked,
// symlinks are refused by Lstat and again at open time (O_NOFOLLOW).
func (a *App) serveAttachMedia(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, mediaAttachPrefix)
	if id == "" || strings.ContainsAny(id, `/\.`) {
		http.NotFound(w, r)
		return
	}
	a.attachMu.Lock()
	path, ok := a.attachMedia[id]
	a.attachMu.Unlock()
	if !ok || !attach.IsImage(path) {
		http.NotFound(w, r)
		return
	}
	if fi, err := os.Lstat(path); err != nil || fi.Mode()&os.ModeSymlink != 0 || !fi.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	f, err := openNoFollow(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	http.ServeContent(w, r, filepath.Base(path), fi.ModTime(), f)
}
