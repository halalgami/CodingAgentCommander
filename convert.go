package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
)

// convertTimeout bounds every external converter: a wedged one must not hang
// an insert.
const convertTimeout = 30 * time.Second

// runConverter runs one external program with a timeout. Arguments are argv
// elements: no shell, nothing interpolated.
func runConverter(name string, args ...string) error {
	return runConverterEnv(nil, name, args...)
}

// runConverterEnv adds extra environment entries to the child's own.
func runConverterEnv(env []string, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), convertTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("%s timed out after %s", name, convertTimeout)
	}
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if len(msg) > 300 {
			msg = msg[:300]
		}
		if msg == "" {
			return fmt.Errorf("%s: %w", name, err)
		}
		return fmt.Errorf("%s: %w: %s", name, err, msg)
	}
	return nil
}

// Exit codes the WIC script uses, so "could not open the file" (locked,
// permissions) is never reported as "no HEIF decoder installed".
const (
	wicExitNoDecoder = 3
	wicExitOpenFail  = 4
)

// A fixed script. The file names are NEVER part of it or of the command line:
// PowerShell would parse them as code ("x$(calc).bmp", "a;iex(...);.tif").
// They arrive in the child's environment and are only ever read as values.
const wicScript = `$ErrorActionPreference='Stop'
Add-Type -AssemblyName PresentationCore
$s=$env:CC_SRC; $d=$env:CC_DST; $f=$env:CC_FMT; $m=[int]$env:CC_MAX
$in=$null
try {
  try { $in=[IO.File]::OpenRead($s) } catch { exit 4 }
  try { $dec=[Windows.Media.Imaging.BitmapDecoder]::Create($in,'PreservePixelFormat','OnLoad') } catch { exit 3 }
  $fr=$dec.Frames[0]; $bmp=$fr
  $max=[Math]::Max($fr.PixelWidth,$fr.PixelHeight)
  if ($max -gt $m) { $k=[double]$m/$max; $bmp=New-Object Windows.Media.Imaging.TransformedBitmap($fr,(New-Object Windows.Media.ScaleTransform($k,$k))) }
  if ($f -eq 'jpeg') { $enc=New-Object Windows.Media.Imaging.JpegBitmapEncoder; $enc.QualityLevel=90 } else { $enc=New-Object Windows.Media.Imaging.PngBitmapEncoder }
  $enc.Frames.Add([Windows.Media.Imaging.BitmapFrame]::Create($bmp))
  $out=[IO.File]::Create($d)
  try { $enc.Save($out) } finally { $out.Close() }
} finally { if ($in) { $in.Dispose() } }`

// encodePowerShell is the -EncodedCommand form: base64 of the UTF-16LE text.
func encodePowerShell(script string) string {
	u := utf16.Encode([]rune(script))
	b := make([]byte, 0, len(u)*2)
	for _, c := range u {
		b = append(b, byte(c), byte(c>>8))
	}
	return base64.StdEncoding.EncodeToString(b)
}

// wicInvocation builds the powershell call: argv carries only the fixed
// encoded script, the environment carries every path.
func wicInvocation(src, dst, format string, maxSide int) (args, env []string) {
	return []string{"-NoProfile", "-NonInteractive", "-EncodedCommand", encodePowerShell(wicScript)},
		[]string{"CC_SRC=" + src, "CC_DST=" + dst, "CC_FMT=" + format, "CC_MAX=" + strconv.Itoa(maxSide)}
}

// magickArgs: the source gets an explicit frame suffix so a "[1]" inside the
// file name is part of the name, not a frame spec (ImageMagick reads only the
// LAST bracket group as one), and the shrink-only geometry never enlarges.
func magickArgs(src, dst, format string, maxSide int) []string {
	geom := strconv.Itoa(maxSide) + "x" + strconv.Itoa(maxSide) + ">"
	args := []string{src + "[0]", "-auto-orient", "-resize", geom}
	if format == "jpeg" {
		args = append(args, "-quality", "90")
	}
	return append(args, format+":"+dst)
}
