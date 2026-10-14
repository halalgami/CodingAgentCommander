package main

import (
	"encoding/base64"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestEncodePowerShellIsBase64OfUTF16LE(t *testing.T) {
	raw, err := base64.StdEncoding.DecodeString(encodePowerShell("A€😀"))
	if err != nil || len(raw)%2 != 0 {
		t.Fatalf("%v len %d", err, len(raw))
	}
	u := make([]uint16, len(raw)/2)
	for i := range u {
		u[i] = uint16(raw[2*i]) | uint16(raw[2*i+1])<<8
	}
	if got := string(utf16.Decode(u)); got != "A€😀" {
		t.Errorf("round trip = %q", got)
	}
}

// The injection regression: nothing a file is named may reach the command line.
func TestWICInvocationKeepsEveryPathOutOfArgv(t *testing.T) {
	names := []string{
		`C:\x\x$(Start-Process calc).bmp`, `C:\x\a;iex(1);.tif`, `C:\x\it's.heic`,
		"C:\\x\\back`tick.png", `C:\x\two words.heic`, `C:\x\"quoted".ico`,
	}
	for _, n := range names {
		args, env := wicInvocation(n, n+".out", "jpeg", 8000)
		for _, a := range args {
			if strings.Contains(a, "x$(") || strings.Contains(a, "iex(") || strings.Contains(a, n) || strings.Contains(a, `C:\x`) {
				t.Errorf("%q: argv leaks the path: %q", n, a)
			}
		}
		if len(args) != 4 || args[0] != "-NoProfile" || args[1] != "-NonInteractive" || args[2] != "-EncodedCommand" {
			t.Errorf("argv = %q", args)
		}
		want := map[string]bool{"CC_SRC=" + n: true, "CC_DST=" + n + ".out": true, "CC_FMT=jpeg": true, "CC_MAX=8000": true}
		for _, e := range env {
			delete(want, e)
		}
		if len(want) != 0 {
			t.Errorf("%q: env missing %v (got %q)", n, want, env)
		}
	}
	// and the script itself must not mention any caller data
	if strings.Contains(wicScript, "CC_SRC=") {
		t.Error("the script is fixed text")
	}
}

func TestMagickArgsKeepABracketInTheNameLiteral(t *testing.T) {
	a := magickArgs("/p/photo[1].heic", "/out/convert-x.jpg", "jpeg", 8000)
	if a[0] != "/p/photo[1].heic[0]" || a[len(a)-1] != "jpeg:/out/convert-x.jpg" {
		t.Errorf("args = %q", a)
	}
	for _, x := range a {
		if x == "-resize" {
			continue
		}
	}
	found := false
	for i, x := range a {
		if x == "-resize" && a[i+1] == "8000x8000>" {
			found = true
		}
	}
	if !found {
		t.Errorf("shrink-only geometry missing: %q", a)
	}
}
