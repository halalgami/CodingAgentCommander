package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/halalgami/CodingAgentCommander/internal/attach"
)

// stubClipboard swaps the seam; it never touches the real pasteboard.
func stubClipboard(t *testing.T, f func(destBase string) (clipContents, error)) {
	t.Helper()
	old := clipboardRead
	clipboardRead = f
	t.Cleanup(func() { clipboardRead = old })
}

func stubFiles(t *testing.T, paths []string, err error) {
	stubClipboard(t, func(string) (clipContents, error) { return clipContents{Files: paths}, err })
}

func TestParseClipboard(t *testing.T) {
	c, err := parseClipboard([]byte(`{"files":["/a/my file.pdf","/b/c.png"]}` + "\n"))
	if err != nil || len(c.Files) != 2 || c.Files[0] != "/a/my file.pdf" || c.Image != "" {
		t.Fatalf("got %+v, %v", c, err)
	}
	if c, err := parseClipboard([]byte(`{"files":[],"image":"/x/paste.tiff"}`)); err != nil || c.Image != "/x/paste.tiff" || len(c.Files) != 0 {
		t.Errorf("image: %+v, %v", c, err)
	}
	if c, err := parseClipboard([]byte(`{"files":[]}`)); err != nil || len(c.Files) != 0 || c.Image != "" {
		t.Errorf("empty: %+v, %v", c, err)
	}
	for _, bad := range []string{"", "garbage", `["/a"]`, `{"files":"x"}`} {
		if _, err := parseClipboard([]byte(bad)); err == nil {
			t.Errorf("%q must be an error", bad)
		}
	}
}

func TestPasteClipboardFilesUnsupported(t *testing.T) {
	a, h := newAttachApp(t)
	addSession(a, "@1", "W1", "")
	stubClipboard(t, nil)
	_, err := a.PasteClipboardFiles("@1")
	if err == nil || err.Error() != "Pasting files isn't supported here — drag them onto the session instead" || len(h.pasted) != 0 {
		t.Errorf("err = %v pasted = %q", err, h.pasted)
	}
}

func TestPasteClipboardFilesEmpty(t *testing.T) {
	a, h := newAttachApp(t)
	addSession(a, "@1", "W1", "")
	stubFiles(t, []string{}, nil)
	_, err := a.PasteClipboardFiles("@1")
	if err == nil || err.Error() != "The clipboard has no files" || len(h.pasted) != 0 {
		t.Errorf("err = %v pasted = %q", err, h.pasted)
	}
}

func TestPasteClipboardFilesReaderErrorIsSurfaced(t *testing.T) {
	a, _ := newAttachApp(t)
	addSession(a, "@1", "W1", "")
	boom := errors.New("osascript: boom")
	stubFiles(t, nil, boom)
	if _, err := a.PasteClipboardFiles("@1"); !errors.Is(err, boom) {
		t.Errorf("err = %v", err)
	}
}

func TestPasteClipboardFilesCapsTheBatch(t *testing.T) {
	a, h := newAttachApp(t)
	addSession(a, "@1", "W1", "")
	dir := t.TempDir()
	var ps []string
	for i := 0; i <= maxBatch; i++ {
		ps = append(ps, touch(t, filepath.Join(dir, strings.Repeat("a", i+1)+".txt")))
	}
	stubFiles(t, ps, nil)
	_, err := a.PasteClipboardFiles("@1")
	if err == nil || err.Error() != "Attach at most 20 files at once" || len(h.pasted) != 0 {
		t.Errorf("err = %v pasted = %q", err, h.pasted)
	}
}

func TestPasteClipboardFilesRefusedWhileAwaitingAnswer(t *testing.T) {
	a, h := newAttachApp(t)
	addSession(a, "@2", "W2", "")
	a.sessions["@2"].AwaitingSession = "x"
	p := touch(t, filepath.Join(t.TempDir(), "a.pdf"))
	stubFiles(t, []string{p}, nil)
	_, err := a.PasteClipboardFiles("@2")
	if err == nil || err.Error() != waitingW2 || len(h.pasted) != 0 {
		t.Errorf("err = %v pasted = %q", err, h.pasted)
	}
}

func TestPasteClipboardFilesRejectsControlPathButInsertsTheRest(t *testing.T) {
	a, h := newAttachApp(t)
	addSession(a, "@1", "W1", t.TempDir())
	dir := t.TempDir()
	ok := touch(t, filepath.Join(dir, "ok.pdf"))
	stubFiles(t, []string{filepath.Join(dir, "evil\nname.pdf"), ok}, nil)
	res, err := a.PasteClipboardFiles("@1")
	if err != nil {
		t.Fatal(err)
	}
	if want := "@1\x00" + attach.PromptText(ok, "") + " "; len(h.pasted) != 1 || h.pasted[0] != want {
		t.Errorf("pasted %q, want [%q]", h.pasted, want)
	}
	if len(res.Rejected) != 1 || res.Rejected[0].Reason != attach.ErrControlChar.Error() || len(res.Inserted) != 1 {
		t.Errorf("res = %+v", res)
	}
}

func TestPasteClipboardFilesPastesOnceRelativeEscaped(t *testing.T) {
	a, h := newAttachApp(t)
	proj := t.TempDir()
	in := touch(t, filepath.Join(proj, "docs", "my spec.pdf"))
	addSession(a, "@3", "W3", proj)
	stubFiles(t, []string{in, in}, nil)
	res, err := a.PasteClipboardFiles("@3")
	if err != nil {
		t.Fatal(err)
	}
	want := "@3\x00" + attach.PromptText(in, proj) + " "
	if len(h.pasted) != 1 || h.pasted[0] != want || len(h.sentKeys) != 0 || len(res.Inserted) != 1 {
		t.Errorf("pasted %q want [%q] res %+v", h.pasted, want, res)
	}
}

func TestPasteClipboardImageIsWrittenUnderTheStoreAndInserted(t *testing.T) {
	a, h := newAttachApp(t)
	addSession(a, "@1", "W1", "")
	var base string
	stubClipboard(t, func(b string) (clipContents, error) {
		base = b
		img := writePNG(t, b+".png", 3, 3) // what the native reader would have written
		return clipContents{Image: img}, nil
	})
	stubConvert(t, nil)
	res, err := a.PasteClipboardFiles("@1")
	if err != nil || len(res.Inserted) != 1 || res.Inserted[0].Path != base+".png" {
		t.Fatalf("%v %+v", err, res)
	}
	if filepath.Dir(base) != a.attachStore.Dir || !strings.HasPrefix(filepath.Base(base), "paste-") || len(h.pasted) != 1 {
		t.Errorf("base %s pasted %q", base, h.pasted)
	}
}

func TestPasteClipboardTIFFImageIsConverted(t *testing.T) {
	a, h := newAttachApp(t)
	addSession(a, "@1", "W1", "")
	stubClipboard(t, func(b string) (clipContents, error) {
		os.WriteFile(b+".tiff", []byte("II*\x00"), 0o600)
		return clipContents{Image: b + ".tiff"}, nil
	})
	calls := stubConvert(t, nil)
	res, err := a.PasteClipboardFiles("@1")
	if err != nil || len(*calls) != 1 || (*calls)[0][2] != "png" || len(res.Inserted) != 1 ||
		!strings.HasPrefix(filepath.Base(res.Inserted[0].Path), "convert-") || len(h.pasted) != 1 {
		t.Errorf("%v %+v %v", err, res, *calls)
	}
}

func TestPasteClipboardFilesWinOverAnImage(t *testing.T) {
	a, _ := newAttachApp(t)
	addSession(a, "@1", "W1", "")
	p := touch(t, filepath.Join(t.TempDir(), "a.pdf"))
	stubClipboard(t, func(string) (clipContents, error) { return clipContents{Files: []string{p}, Image: "/nope.png"}, nil })
	if res, err := a.PasteClipboardFiles("@1"); err != nil || len(res.Inserted) != 1 || res.Inserted[0].Path != p {
		t.Errorf("%v %+v", err, res)
	}
}

func TestPasteClipboardRefusesAnUnknownOrWaitingWindowBeforeReadingAnything(t *testing.T) {
	a, _ := newAttachApp(t)
	addSession(a, "@2", "W2", "")
	a.sessions["@2"].AwaitingSession = "x"
	read := false
	stubClipboard(t, func(string) (clipContents, error) { read = true; return clipContents{}, nil })
	if _, err := a.PasteClipboardFiles("@9"); err == nil || err.Error() != "no such session" {
		t.Errorf("unknown: %v", err)
	}
	if _, err := a.PasteClipboardFiles("@2"); err == nil || err.Error() != waitingW2 {
		t.Errorf("waiting: %v", err)
	}
	if read {
		t.Error("the clipboard was read for a window that cannot take the paste")
	}
	if es, _ := os.ReadDir(a.attachStore.Dir); len(es) != 0 {
		t.Errorf("files written for a refused paste: %v", es)
	}
}

func TestPasteClipboardTooBigImageIsRefusedWithItsSize(t *testing.T) {
	a, h := newAttachApp(t)
	addSession(a, "@1", "W1", "")
	stubClipboard(t, func(string) (clipContents, error) { return clipContents{TooBig: 30 << 20}, nil })
	_, err := a.PasteClipboardFiles("@1")
	if err == nil || err.Error() != "the clipboard image is 30 MB, which is too large to paste" || len(h.pasted) != 0 {
		t.Errorf("err = %v", err)
	}
}

// The frontend recognises "Go has nothing to give" by these two messages
// (attachments.svelte.js nativeAbsent). Pin them so renaming one here breaks
// this test instead of silently breaking the fallback.
func TestFrontendAbsentMessagesMatchGo(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("frontend", "src", "lib", "attachments.svelte.js"))
	if err != nil {
		t.Fatal(err)
	}
	js := string(b)
	for _, want := range []string{errClipboardNoFiles.Error(), "Pasting files isn't supported"} {
		if !strings.Contains(js, want) {
			t.Errorf("attachments.svelte.js no longer matches Go's %q", want)
		}
	}
	if !strings.HasPrefix(errClipboardUnsupported.Error(), "Pasting files isn't supported") {
		t.Error("errClipboardUnsupported changed its prefix")
	}
}
