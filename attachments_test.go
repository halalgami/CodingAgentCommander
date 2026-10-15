package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/halalgami/CodingAgentCommander/internal/attach"
)

// newAttachApp builds on NewApp so handleNotify/pushCompanion (Task 5) have the
// emitter and notifier they need.
func newAttachApp(t *testing.T) (*App, *captureHost) {
	t.Helper()
	h := &captureHost{}
	a := NewApp()
	a.host = h
	a.notifier = &fakeNotifier{}
	a.emitter = &fakeEmitter{}
	a.attachStore = &attach.Store{Dir: filepath.Join(t.TempDir(), "attachments")}
	return a, h
}

func addSession(a *App, id, name, cwd string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sessions[id] = &sessionRec{Name: name, Cwd: cwd, Status: "active"}
}

func touch(t *testing.T, p string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestInsertPathsPastesOnceRelativeEscapedNoEnter(t *testing.T) {
	a, h := newAttachApp(t)
	proj := t.TempDir()
	in := touch(t, filepath.Join(proj, "docs", "my spec.pdf"))
	img := touch(t, filepath.Join(proj, "shot.png"))
	addSession(a, "@3", "W3", proj)

	res, err := a.InsertPaths("@3", []string{in, img})
	if err != nil {
		t.Fatal(err)
	}
	want := "@3\x00" + attach.PromptText(in, proj) + " " + attach.PromptText(img, proj) + " "
	if runtime.GOOS != "windows" && want != "@3\x00"+`./docs/my\ spec.pdf ./shot.png ` {
		t.Fatalf("PromptText drifted: %q", want)
	}
	if len(h.pasted) != 1 || h.pasted[0] != want {
		t.Errorf("pasted %q, want [%q]", h.pasted, want)
	}
	if len(h.sentKeys) != 0 {
		t.Error("must never use SendKeys (it presses Enter)")
	}
	if len(res.Inserted) != 2 || res.Inserted[0].Path != in || res.Inserted[0].Name != "my spec.pdf" ||
		res.Inserted[0].Kind != "file" || res.Inserted[0].Thumb != "" ||
		res.Inserted[1].Kind != "image" || res.Inserted[1].Thumb != "/media/attach/"+res.Inserted[1].ID ||
		len(res.Inserted[1].ID) != 24 {
		t.Errorf("inserted = %+v", res.Inserted)
	}
}

func TestInsertPathsRefusesBadNamesButInsertsTheRest(t *testing.T) {
	a, h := newAttachApp(t)
	dir := t.TempDir()
	addSession(a, "@1", "W1", t.TempDir()) // cwd elsewhere: paths stay absolute
	ok := touch(t, filepath.Join(dir, "ok.pdf"))
	res, err := a.InsertPaths("@1", []string{filepath.Join(dir, "evil name.pdf"), ok, filepath.Join(dir, "gone.txt"), "rel.png"})
	if err != nil {
		t.Fatal(err)
	}
	if want := "@1\x00" + attach.PromptText(ok, "") + " "; len(h.pasted) != 1 || h.pasted[0] != want {
		t.Errorf("pasted %q, want [%q]", h.pasted, want)
	}
	want := []Rejection{
		{Name: "evil?name.pdf", Reason: attach.ErrControlChar.Error()},
		{Name: "gone.txt", Reason: attach.ErrMissing.Error()},
		{Name: "rel.png", Reason: attach.ErrNotAbsolute.Error()},
	}
	if fmt.Sprint(res.Rejected) != fmt.Sprint(want) {
		t.Errorf("rejected = %+v\nwant       %+v", res.Rejected, want)
	}
}

func TestInsertPathsUnknownWindowIsRefused(t *testing.T) {
	a, h := newAttachApp(t)
	_, err := a.InsertPaths("@9", []string{touch(t, filepath.Join(t.TempDir(), "a.pdf"))})
	if err == nil || err.Error() != "no such session" || len(h.pasted) != 0 {
		t.Errorf("err = %v, pasted = %q", err, h.pasted)
	}
}

func TestInsertPathsDedupesKeepingFirst(t *testing.T) {
	a, h := newAttachApp(t)
	addSession(a, "@1", "W1", "")
	p := touch(t, filepath.Join(t.TempDir(), "a.png"))
	sep := string(filepath.Separator)
	unclean := filepath.Dir(p) + sep + "." + sep + "a.png" // the same file, spelled differently
	res, err := a.InsertPaths("@1", []string{p, unclean, p})
	if err != nil || len(res.Inserted) != 1 || h.pasted[0] != "@1\x00"+attach.PromptText(p, "")+" " {
		t.Errorf("res=%+v err=%v pasted=%q", res, err, h.pasted)
	}
}

func TestInsertPathsCapsTheBatch(t *testing.T) {
	a, h := newAttachApp(t)
	addSession(a, "@1", "W1", "")
	dir := t.TempDir()
	var paths []string
	for i := 0; i < 21; i++ {
		paths = append(paths, touch(t, filepath.Join(dir, fmt.Sprintf("f%d.pdf", i))))
	}
	if _, err := a.InsertPaths("@1", paths); err == nil || err.Error() != "Attach at most 20 files at once" || len(h.pasted) != 0 {
		t.Errorf("21 paths: err = %v, pasted = %d", err, len(h.pasted))
	}
	if _, err := a.InsertPaths("@1", paths[:20]); err != nil || len(h.pasted) != 1 {
		t.Errorf("20 paths: err = %v, pasted = %d", err, len(h.pasted))
	}
}

func TestInsertPathsAllRejectedPastesNothing(t *testing.T) {
	a, h := newAttachApp(t)
	addSession(a, "@1", "W1", "")
	res, err := a.InsertPaths("@1", []string{"/nope/a\nb"})
	if err != nil || len(h.pasted) != 0 || len(res.Inserted) != 0 || len(res.Rejected) != 1 {
		t.Errorf("res=%+v err=%v pasted=%q", res, err, h.pasted)
	}
}

func TestInsertPathsPasteFailureClaimsNothing(t *testing.T) {
	a, h := newAttachApp(t)
	addSession(a, "@9", "W9", "")
	// The real shape of Task 3's error for a window that died (tmux 3.7b).
	h.pasteErr = errors.New("tmux paste-buffer: can't find window: @9: exit status 1")
	res, err := a.InsertPaths("@9", []string{touch(t, filepath.Join(t.TempDir(), "a.pdf"))})
	if err == nil || err.Error() != "could not insert into W9: tmux paste-buffer: can't find window: @9: exit status 1" {
		t.Fatalf("err = %v", err)
	}
	if len(res.Inserted) != 0 {
		t.Error("Inserted must be empty when the paste failed")
	}
}

func TestInsertPathsNeverReturnsNilSlices(t *testing.T) {
	a, _ := newAttachApp(t)
	addSession(a, "@1", "W1", "")
	res, _ := a.InsertPaths("@1", nil)
	if res.Inserted == nil || res.Rejected == nil {
		t.Error("nil slice marshals as null and breaks the frontend's .length")
	}
}

func TestInsertDroppedPathsInsertsAGoodDrop(t *testing.T) {
	a, h := newAttachApp(t)
	addSession(a, "@2", "W2", "")
	dir := t.TempDir()
	p1, p2 := touch(t, filepath.Join(dir, "a.pdf")), touch(t, filepath.Join(dir, "b.pdf"))
	res, err := a.InsertDroppedPaths("@2", []string{p1, p2})
	want := "@2\x00" + attach.PromptText(p1, "") + " " + attach.PromptText(p2, "") + " "
	if err != nil || len(res.Inserted) != 2 || len(h.pasted) != 1 || h.pasted[0] != want {
		t.Errorf("res=%+v err=%v pasted=%q", res, err, h.pasted)
	}
}

const brokenDrop = "A dropped name contains a line break or was moved — nothing was inserted."

// Wails delivers "<d>/ok.pdf" and "<d>/evil\nname.pdf" as THREE entries.
func TestInsertDroppedPathsRefusesWholeSplitDrop(t *testing.T) {
	a, h := newAttachApp(t)
	addSession(a, "@1", "W1", "")
	dir := t.TempDir()
	ok := touch(t, filepath.Join(dir, "ok.pdf"))
	_, err := a.InsertDroppedPaths("@1", []string{ok, filepath.Join(dir, "evil"), "name.pdf"})
	if err == nil || err.Error() != brokenDrop || len(h.pasted) != 0 {
		t.Errorf("err = %v, pasted = %q (ok.pdf must NOT go in alone)", err, h.pasted)
	}
}

// A directory named "x\n" splits into "/dl/x" and an absolute path that may be
// real (here: a stand-in for ~/.ssh/id_rsa). With "/dl/x" missing, the
// missing-fragment rule refuses the whole drop.
func TestInsertDroppedPathsRefusesSplitDirectoryWithMissingFragment(t *testing.T) {
	a, h := newAttachApp(t)
	addSession(a, "@1", "W1", "")
	root := t.TempDir()
	key := touch(t, filepath.Join(root, "home", ".ssh", "id_rsa"))
	_, err := a.InsertDroppedPaths("@1", []string{filepath.Join(root, "dl", "x"), key})
	if err == nil || err.Error() != brokenDrop || len(h.pasted) != 0 {
		t.Errorf("err = %v, pasted = %q", err, h.pasted)
	}
}

// The hard case: an attacker ships BOTH "dl/x" and "dl/x\n<abs path>", so every
// fragment exists and is absolute. Only re-joining the fragments and finding
// the joined path on disk exposes the split.
func TestInsertDroppedPathsRefusesSplitWhoseFragmentsAllExist(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a newline is not legal in a Windows file name")
	}
	a, h := newAttachApp(t)
	addSession(a, "@1", "W1", "")
	root := t.TempDir()
	key := touch(t, filepath.Join(root, "home", ".ssh", "id_rsa"))
	touch(t, filepath.Join(root, "dl", "x"))
	real := touch(t, filepath.Join(root, "dl", "x\n"+key)) // the file the user actually dropped
	frags := strings.Split(real, "\n")
	if len(frags) != 2 || frags[1] != key {
		t.Fatalf("fixture did not split as Wails would: %q", frags)
	}
	_, err := a.InsertDroppedPaths("@1", frags)
	if err == nil || err.Error() != brokenDrop || len(h.pasted) != 0 {
		t.Errorf("err = %v, pasted = %q", err, h.pasted)
	}
}

// A text or URL drag (Safari link) arrives as [""].
func TestInsertDroppedPathsTextDragIsNotAFileDrop(t *testing.T) {
	a, h := newAttachApp(t)
	addSession(a, "@1", "W1", "")
	_, err := a.InsertDroppedPaths("@1", []string{""})
	if err == nil || err.Error() != "Only files can be dropped onto a session." || len(h.pasted) != 0 {
		t.Errorf("err = %v, pasted = %q", err, h.pasted)
	}
}

func TestInsertDroppedPathsCapsTheDrop(t *testing.T) {
	a, h := newAttachApp(t)
	addSession(a, "@1", "W1", "")
	dir := t.TempDir()
	var paths []string
	for i := 0; i < 21; i++ {
		paths = append(paths, touch(t, filepath.Join(dir, fmt.Sprintf("f%d.pdf", i))))
	}
	if _, err := a.InsertDroppedPaths("@1", paths); err == nil || err.Error() != "Drop at most 20 files at once" || len(h.pasted) != 0 {
		t.Errorf("err = %v, pasted = %d", err, len(h.pasted))
	}
}

func TestSaveClipboardImage(t *testing.T) {
	a, _ := newAttachApp(t)
	p, err := a.SaveClipboardImage(base64.StdEncoding.EncodeToString([]byte("\x89PNG")), "image/png")
	if err != nil || filepath.Dir(p) != a.attachStore.Dir || filepath.Ext(p) != ".png" {
		t.Errorf("SaveClipboardImage = %q, %v", p, err)
	}
	if _, err := a.SaveClipboardImage("not base64!!", "image/png"); err == nil || err.Error() != "the clipboard image could not be read" {
		t.Errorf("bad base64: %v", err)
	}
	if _, err := a.SaveClipboardImage(base64.StdEncoding.EncodeToString([]byte("x")), "image/x-foo"); err == nil ||
		err.Error() != "image/x-foo images are not supported — use PNG, JPEG, GIF or WebP" {
		t.Errorf("unknown format: %v", err)
	}
}

// Traversal ids are tested on the handler itself: http.ServeMux cleans
// "/media/attach/../../etc/passwd" and answers with a redirect before our code
// ever runs, so a mux-level test would pass even with no guard at all.
func TestServeAttachMediaRefusesNonIDs(t *testing.T) {
	a, _ := newAttachApp(t)
	// A real image one level above the attachments folder: served only if an
	// id were ever treated as a path.
	secret := filepath.Join(filepath.Dir(a.attachStore.Dir), "secret.png")
	if err := os.WriteFile(secret, []byte("SECRET"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/media/attach/", "/media/attach/unknown", "/media/attach/../secret.png", "/media/attach/../../etc/passwd", `/media/attach/..\..\x`, "/media/attach/a/b"} {
		rec := httptest.NewRecorder()
		a.serveAttachMedia(rec, httptest.NewRequest("GET", "http://wails.localhost"+strings.ReplaceAll(path, `\`, "%5C"), nil))
		if rec.Code != 404 {
			t.Errorf("%s served %d", path, rec.Code)
		}
	}
}

func TestAttachMediaServesRegisteredImagesOnly(t *testing.T) {
	a, _ := newAttachApp(t)
	addSession(a, "@1", "W1", "")
	img := filepath.Join(t.TempDir(), "a.png")
	if err := os.WriteFile(img, []byte("PNGDATA"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := a.InsertPaths("@1", []string{img})
	if err != nil {
		t.Fatal(err)
	}
	h := a.packMediaHandler()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", res.Inserted[0].Thumb, nil))
	if rec.Code != 200 || rec.Body.String() != "PNGDATA" {
		t.Errorf("registered image: %d %q", rec.Code, rec.Body.String())
	}
	if runtime.GOOS == "windows" {
		return // symlinks need privilege on Windows
	}
	// Replace the stored (symlink-resolved) file with a symlink to elsewhere.
	real, _ := filepath.EvalSymlinks(img)
	if err := os.Remove(real); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/hosts", real); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", res.Inserted[0].Thumb, nil))
	if rec.Code != 404 {
		t.Errorf("symlink served: %d", rec.Code)
	}
}

func TestPasteFailureMessageNamesTheRefusedFiles(t *testing.T) {
	a, h := newAttachApp(t)
	addSession(a, "@1", "W1", "")
	h.pasteErr = errors.New("tmux paste-buffer: can't find window: @1: exit status 1")
	ok := touch(t, filepath.Join(t.TempDir(), "ok.pdf"))
	_, err := a.InsertPaths("@1", []string{ok, filepath.Join(t.TempDir(), "nope", "gone.txt")})
	want := "could not insert into W1: tmux paste-buffer: can't find window: @1: exit status 1; also refused: gone.txt (" + attach.ErrMissing.Error() + ")"
	if err == nil || err.Error() != want {
		t.Errorf("err = %v\nwant  %s", err, want)
	}
}
