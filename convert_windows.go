//go:build windows

package main

import (
	"errors"
	"os/exec"

	"github.com/halalgami/CodingAgentCommander/internal/attach"
)

var convertImage = func(src, dst, format string, maxSide int) error {
	args, env := wicInvocation(src, dst, format, maxSide)
	err := runConverterEnv(env, "powershell", args...)
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		switch ee.ExitCode() {
		case wicExitNoDecoder:
			return &attach.NoConverterError{Hint: "Windows could not decode it; for HEIC/AVIF install the HEIF Image Extensions (and HEVC Video Extensions) from the Microsoft Store"}
		case wicExitOpenFail:
			return errors.New("the file could not be opened (is it locked by another program?)")
		}
	}
	return err
}
