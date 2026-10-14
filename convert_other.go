//go:build !darwin && !windows

package main

import (
	"os/exec"
	"strings"

	"github.com/halalgami/CodingAgentCommander/internal/attach"
)

const convertHint = "install ImageMagick (with HEIF support) or libheif-examples (heif-convert)"

// convertImage prefers ImageMagick (reads every format, shrinks with a
// shrink-only geometry). A HEIC/HEIF that ImageMagick cannot read (no HEIF
// delegate) falls back to heif-convert, which cannot resize; Normalize then
// checks the result's size. Nothing found is a NoConverterError.
var convertImage = func(src, dst, format string, maxSide int) error {
	low := strings.ToLower(src)
	heif := strings.HasSuffix(low, ".heic") || strings.HasSuffix(low, ".heif")
	var firstErr error
	for _, bin := range []string{"magick", "convert"} {
		if p, err := exec.LookPath(bin); err == nil {
			if err := runConverter(p, magickArgs(src, dst, format, maxSide)...); err == nil {
				return nil
			} else if firstErr == nil {
				firstErr = err
			}
			break
		}
	}
	if heif {
		if p, err := exec.LookPath("heif-convert"); err == nil {
			return runConverter(p, "-q", "90", src, dst)
		}
	}
	if firstErr != nil {
		return firstErr
	}
	return &attach.NoConverterError{Hint: convertHint}
}
