//go:build windows && bundled

package deps

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

// PsmuxVersion is the psmux release the embedded binary comes from. build.ps1
// downloads exactly this version, hash-pinned, into assets/ before building.
const PsmuxVersion = "3.3.8"

// The binary is fetched by the build script rather than committed: 7 MB in git
// bloats every clone forever, and pinning the download by SHA256 in build.ps1
// buys the same reproducibility without the weight.
//
// That is also why this file sits behind the `bundled` tag. go:embed fails at
// compile time when a pattern matches nothing, so an unconditional embed would
// break `go build ./...` on any clone that has not run the asset step — the same
// trap the release workflow already documents for frontend/dist.
//
//go:embed assets/tmux.exe
//go:embed assets/psmux-LICENSE
var psmuxAssets embed.FS

// PsmuxBundled reports whether this build carries its own psmux.
func PsmuxBundled() bool { return true }

// EnsureBundledPsmux extracts the embedded psmux into the managed tool dir and
// returns the directory to add to PATH. It is idempotent: an already-extracted
// copy of the expected size is left untouched, so this costs one Stat on all but
// the first launch.
//
// The directory is version-scoped (bin\psmux-3.3.8) for a Windows-specific
// reason: a running psmux server holds tmux.exe open, and Windows refuses to
// replace a file that is in use. Giving each version its own directory means an
// upgrade never overwrites a locked binary — the old copy just stops being on
// PATH, and the running server that owns it keeps working until it exits.
func EnsureBundledPsmux() (string, error) {
	dir := filepath.Join(ManagedDir(), "bin", "psmux-"+PsmuxVersion)
	exe := filepath.Join(dir, "tmux.exe")

	data, err := psmuxAssets.ReadFile("assets/tmux.exe")
	if err != nil {
		return "", fmt.Errorf("read embedded psmux: %w", err)
	}
	if fi, err := os.Stat(exe); err == nil && fi.Size() == int64(len(data)) {
		return dir, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create %s: %w", dir, err)
	}
	if err := writeAtomic(exe, data, 0o755); err != nil {
		return "", fmt.Errorf("extract psmux: %w", err)
	}
	// psmux is MIT-licensed; ship the notice beside the binary we redistribute.
	if lic, err := psmuxAssets.ReadFile("assets/psmux-LICENSE"); err == nil {
		_ = writeAtomic(filepath.Join(dir, "LICENSE-psmux.txt"), lic, 0o644)
	}
	return dir, nil
}
