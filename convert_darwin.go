//go:build darwin

package main

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
)

// Trade-off: HEIC/AVIF go to JPEG, which has no alpha, so a transparent source
// is flattened (HEIC photos almost never carry alpha). TIFF/BMP/ICO, where
// alpha is common, go to PNG.
//
// convertImage uses sips, which ships with macOS and reads HEIC, TIFF, BMP,
// ICO and (macOS 13+) AVIF. -Z both shrinks and ENLARGES, so it is passed only
// when the source really is larger than maxSide.
var convertImage = func(src, dst, format string, maxSide int) error {
	args := []string{"-s", "format", format}
	if w, h := sipsSize(src); w > maxSide || h > maxSide {
		args = append(args, "-Z", strconv.Itoa(maxSide))
	}
	args = append(args, src, "--out", dst)
	return runConverter("sips", args...)
}

// sipsSize reads the pixel size; 0,0 when it cannot be told (no downscale is
// attempted then, and the 20 MB cap on the output still applies).
func sipsSize(src string) (w, h int) {
	ctx, cancel := context.WithTimeout(context.Background(), convertTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "sips", "-g", "pixelWidth", "-g", "pixelHeight", src).Output()
	if err != nil {
		return 0, 0
	}
	for _, l := range strings.Split(string(out), "\n") {
		f := strings.Fields(l)
		if len(f) != 2 {
			continue
		}
		n, _ := strconv.Atoi(f[1])
		switch f[0] {
		case "pixelWidth:":
			w = n
		case "pixelHeight:":
			h = n
		}
	}
	return w, h
}
