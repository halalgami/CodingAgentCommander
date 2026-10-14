package main

import (
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/halalgami/CodingAgentCommander/internal/attach"
)

// stubConvert swaps the platform converter; it records calls and writes a
// stand-in output.
func stubConvert(t *testing.T, f func(src, dst, format string, max int) error) *[][3]string {
	t.Helper()
	var calls [][3]string
	old := convertImage
	convertImage = func(src, dst, format string, max int) error {
		calls = append(calls, [3]string{src, dst, format})
		if f != nil {
			return f(src, dst, format, max)
		}
		return writeOut(dst, format, 1, 1)
	}
	t.Cleanup(func() { convertImage = old })
	return &calls
}

// writeOut writes a real tiny image in the requested format (Normalize reads
// the converted size back).
func writeOut(dst, format string, w, h int) error {
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	if format == "jpeg" {
		return jpeg.Encode(f, image.NewGray(image.Rect(0, 0, w, h)), nil)
	}
	return png.Encode(f, image.NewGray(image.Rect(0, 0, w, h)))
}

func writePNG(t *testing.T, p string, w, h int) string {
	t.Helper()
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, image.NewGray(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return p
}

var heicHead = append([]byte{0, 0, 0, 24}, []byte("ftypheic")...)

func TestInsertConvertsAnUnsupportedImageAndInsertsTheConvertedPath(t *testing.T) {
	for _, entry := range []struct {
		name string
		call func(a *App, p string) (AttachResult, error)
	}{
		{"InsertPaths", func(a *App, p string) (AttachResult, error) { return a.InsertPaths("@1", []string{p}) }},
		{"InsertDroppedPaths", func(a *App, p string) (AttachResult, error) { return a.InsertDroppedPaths("@1", []string{p}) }},
	} {
		a, h := newAttachApp(t)
		addSession(a, "@1", "W1", t.TempDir())
		src := touch(t, filepath.Join(t.TempDir(), "IMG_0001.HEIC"))
		os.WriteFile(src, heicHead, 0o600)
		calls := stubConvert(t, nil)
		res, err := entry.call(a, src)
		if err != nil || len(res.Inserted) != 1 {
			t.Fatalf("%s: %v %+v", entry.name, err, res)
		}
		got := res.Inserted[0].Path
		if filepath.Dir(got) != a.attachStore.Dir || !strings.HasPrefix(filepath.Base(got), "convert-") || filepath.Ext(got) != ".jpg" {
			t.Errorf("%s: inserted %s", entry.name, got)
		}
		if len(*calls) != 1 || (*calls)[0][0] != src || (*calls)[0][2] != "jpeg" {
			t.Errorf("%s: calls %v", entry.name, *calls)
		}
		if len(h.pasted) != 1 || !strings.Contains(h.pasted[0], got) || strings.Contains(h.pasted[0], "HEIC") {
			t.Errorf("%s: pasted %q must carry the converted path only", entry.name, h.pasted)
		}
		if res.Inserted[0].Kind != "image" || res.Inserted[0].Thumb == "" {
			t.Errorf("%s: the converted file is an image with a thumbnail: %+v", entry.name, res.Inserted[0])
		}
	}
}

func TestInsertPassesSupportedFilesThroughWithoutTheConverter(t *testing.T) {
	a, h := newAttachApp(t)
	addSession(a, "@1", "W1", "")
	dir := t.TempDir()
	img := writePNG(t, filepath.Join(dir, "a.png"), 4, 4)
	pdf := touch(t, filepath.Join(dir, "a.pdf"))
	calls := stubConvert(t, nil)
	res, err := a.InsertPaths("@1", []string{img, pdf})
	if err != nil || len(res.Inserted) != 2 || res.Inserted[0].Path != img || len(*calls) != 0 {
		t.Fatalf("%v %+v calls=%v", err, res, *calls)
	}
	if len(h.pasted) != 1 {
		t.Errorf("pasted %q", h.pasted)
	}
}

func TestInsertRefusesAnOversizeImageFromAnySource(t *testing.T) {
	a, h := newAttachApp(t)
	addSession(a, "@1", "W1", "")
	big := filepath.Join(t.TempDir(), "big.png")
	writePNG(t, big, 4, 4)
	f, _ := os.OpenFile(big, os.O_APPEND|os.O_WRONLY, 0)
	f.Write(make([]byte, attach.MaxImageBytes))
	f.Close()
	stubConvert(t, nil)
	res, err := a.InsertDroppedPaths("@1", []string{big})
	if err != nil || len(res.Inserted) != 0 || len(res.Rejected) != 1 || !strings.Contains(res.Rejected[0].Reason, "the limit is 20 MB") || len(h.pasted) != 0 {
		t.Errorf("%v %+v %q", err, res, h.pasted)
	}
}

func TestInsertDownscalesAnOverlongImage(t *testing.T) {
	a, _ := newAttachApp(t)
	addSession(a, "@1", "W1", "")
	wide := writePNG(t, filepath.Join(t.TempDir(), "wide.png"), 8001, 1)
	calls := stubConvert(t, nil)
	res, err := a.InsertPaths("@1", []string{wide})
	if err != nil || len(res.Inserted) != 1 || len(*calls) != 1 || (*calls)[0][2] != "png" || res.Inserted[0].Path == wide {
		t.Errorf("%v %+v %v", err, res, *calls)
	}
}

func TestInsertConverterFailureIsAPerFileRejectionAndOthersGoIn(t *testing.T) {
	a, h := newAttachApp(t)
	addSession(a, "@1", "W1", "")
	dir := t.TempDir()
	bad := filepath.Join(dir, "IMG.heic")
	os.WriteFile(bad, heicHead, 0o600)
	ok := touch(t, filepath.Join(dir, "ok.pdf"))
	stubConvert(t, func(_, _, _ string, _ int) error {
		return &attach.NoConverterError{Hint: "install libheif"}
	})
	res, err := a.InsertPaths("@1", []string{bad, ok})
	if err != nil || len(res.Inserted) != 1 || res.Inserted[0].Path != ok {
		t.Fatalf("%v %+v", err, res)
	}
	if len(res.Rejected) != 1 || res.Rejected[0].Name != "IMG.heic" ||
		res.Rejected[0].Reason != "HEIC images need conversion and no converter is available — install libheif" {
		t.Errorf("rejected %+v", res.Rejected)
	}
	if len(h.pasted) != 1 || strings.Contains(h.pasted[0], "IMG") {
		t.Errorf("pasted %q", h.pasted)
	}
}

func TestInsertConvertsADuplicatedPathOnce(t *testing.T) {
	a, _ := newAttachApp(t)
	addSession(a, "@1", "W1", "")
	src := filepath.Join(t.TempDir(), "a.tiff")
	os.WriteFile(src, []byte("II*\x00"), 0o600)
	calls := stubConvert(t, nil)
	if res, err := a.InsertPaths("@1", []string{src, src}); err != nil || len(res.Inserted) != 1 || len(*calls) != 1 {
		t.Errorf("%v %+v %v", err, res, *calls)
	}
}

func TestInsertRefusesWhenTheSessionStartsWaitingMidConversion(t *testing.T) {
	a, h := newAttachApp(t)
	addSession(a, "@2", "W2", "")
	src := filepath.Join(t.TempDir(), "IMG.heic")
	os.WriteFile(src, heicHead, 0o600)
	stubConvert(t, func(_, dst, format string, _ int) error {
		a.mu.Lock()
		a.sessions["@2"].AwaitingSession = "x" // a permission prompt appeared meanwhile
		a.mu.Unlock()
		return writeOut(dst, format, 1, 1)
	})
	res, err := a.InsertPaths("@2", []string{src})
	if err == nil || err.Error() != waitingW2 || len(h.pasted) != 0 || len(res.Inserted) != 0 {
		t.Errorf("err = %v pasted = %q res = %+v", err, h.pasted, res)
	}
}

func TestInsertRefusesAConverterThatIgnoredTheSizeLimit(t *testing.T) {
	a, h := newAttachApp(t)
	addSession(a, "@1", "W1", "")
	src := filepath.Join(t.TempDir(), "IMG.heic")
	os.WriteFile(src, heicHead, 0o600)
	stubConvert(t, func(_, dst, format string, _ int) error { return writeOut(dst, format, 8001, 1) })
	res, err := a.InsertPaths("@1", []string{src})
	if err != nil || len(res.Inserted) != 0 || len(res.Rejected) != 1 || !strings.Contains(res.Rejected[0].Reason, "over the 8000 px limit") || len(h.pasted) != 0 {
		t.Errorf("%v %+v %q", err, res, h.pasted)
	}
}

func TestConvertedAttachmentCarriesTheOriginalName(t *testing.T) {
	a, _ := newAttachApp(t)
	addSession(a, "@1", "W1", "")
	dir := t.TempDir()
	src := filepath.Join(dir, "IMG_0001.HEIC")
	os.WriteFile(src, heicHead, 0o600)
	plain := writePNG(t, filepath.Join(dir, "plain.png"), 2, 2)
	stubConvert(t, nil)
	res, err := a.InsertPaths("@1", []string{src, plain})
	if err != nil || len(res.Inserted) != 2 {
		t.Fatalf("%v %+v", err, res)
	}
	if res.Inserted[0].OriginalName != "IMG_0001.HEIC" || !strings.HasPrefix(res.Inserted[0].Name, "convert-") || res.Inserted[1].OriginalName != "" {
		t.Errorf("%+v", res.Inserted)
	}
}
