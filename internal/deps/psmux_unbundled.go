//go:build !(windows && bundled)

package deps

import "errors"

// ErrNotBundled is returned by EnsureBundledPsmux in builds that carry no
// embedded psmux: every macOS build, and any Windows build made without
// `-tags bundled` (a plain `go build`, `wails dev`, or `go test`).
var ErrNotBundled = errors.New("this build does not bundle psmux")

// PsmuxBundled reports whether this build carries its own psmux.
func PsmuxBundled() bool { return false }

// EnsureBundledPsmux is the no-op counterpart of the bundled implementation.
// Callers treat the error as "nothing to add to PATH" and fall back to whatever
// the user installed, so a dev build behaves exactly as it did before bundling
// existed.
func EnsureBundledPsmux() (string, error) { return "", ErrNotBundled }
