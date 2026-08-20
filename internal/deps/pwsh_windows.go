//go:build windows

package deps

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// PowerShell 7 is fetched on demand rather than embedded. The self-contained
// win-x64 zip is ~106 MB; folding that into a 17 MB portable exe would make the
// download eight times larger for every user, including the many who already
// have pwsh — and a bigger unsigned binary is worse on the one axis that already
// hurts here, SmartScreen and antivirus friction.
//
// So this mirrors the LiteLLM runtime pattern (internal/router/runtime.go): a
// pinned download into a per-user managed dir, driven by an in-app button, with
// progress streamed to the UI.
const PwshVersion = "7.6.5"

// pwshZipSHA256 pins the release archive per architecture. The upstream hashes
// are published as hashes.sha256 alongside each PowerShell release; verifying
// against a pin in our source — rather than a hash file downloaded from the same
// host as the zip — is what makes this a real integrity check.
var pwshZipSHA256 = map[string]string{
	"amd64": "32eb8f6cdce08f86e987d625a2733e54ac3e289ae7e1621b14c0b5bcec2434ea",
	"arm64": "20514a755d16428dc4355c85e0883c859531e71cc3e122670aa1fccdbf96ba7e",
}

// pwshArch maps Go's arch names onto PowerShell's asset names.
var pwshArch = map[string]string{"amd64": "x64", "arm64": "arm64"}

// ManagedPwshDir is where a fetched PowerShell 7 lives. Version-scoped for the
// same reason as psmux: a pwsh that psmux is running holds its files open, so a
// version bump must never try to overwrite them in place.
func ManagedPwshDir() string {
	return filepath.Join(ManagedDir(), "runtime", "pwsh-"+PwshVersion)
}

// ManagedPwsh is the pwsh executable inside the managed runtime.
func ManagedPwsh() string { return filepath.Join(ManagedPwshDir(), "pwsh.exe") }

// pwshAsset returns the release asset name and its pinned hash for this machine.
func pwshAsset() (name, sha string, err error) {
	arch, ok := pwshArch[runtime.GOARCH]
	if !ok {
		return "", "", fmt.Errorf("no PowerShell 7 build for GOARCH=%s", runtime.GOARCH)
	}
	sha, ok = pwshZipSHA256[runtime.GOARCH]
	if !ok || sha == "" {
		return "", "", fmt.Errorf("no pinned PowerShell %s hash for GOARCH=%s", PwshVersion, runtime.GOARCH)
	}
	return fmt.Sprintf("PowerShell-%s-win-%s.zip", PwshVersion, arch), sha, nil
}

// InstallPwsh downloads the pinned PowerShell 7 archive and extracts it into
// ManagedPwshDir, reporting progress line-by-line through onLine. ctx
// cancellation aborts the download.
//
// It is a no-op when the managed copy is already present, so a retry after a
// partial failure is safe and a second click costs nothing.
func InstallPwsh(ctx context.Context, onLine func(string)) error {
	emit := func(format string, a ...any) {
		if onLine != nil {
			onLine(fmt.Sprintf(format, a...))
		}
	}
	if isExecFile(ManagedPwsh()) {
		emit("PowerShell %s already installed at %s", PwshVersion, ManagedPwsh())
		return nil
	}
	asset, wantSHA, err := pwshAsset()
	if err != nil {
		return err
	}
	url := fmt.Sprintf("https://github.com/PowerShell/PowerShell/releases/download/v%s/%s", PwshVersion, asset)

	// Staging lives under the managed dir, not the OS temp dir: a ~106 MB
	// download plus its extraction wants to land on the same volume as the final
	// location so the closing rename is a rename and not a cross-device copy.
	staging := filepath.Join(ManagedDir(), "runtime", ".staging-pwsh-"+PwshVersion)
	if err := os.MkdirAll(staging, 0o755); err != nil {
		return fmt.Errorf("create staging dir: %w", err)
	}
	defer os.RemoveAll(staging) // best-effort; a crash leaves a .staging- dir we overwrite next time

	zipPath := filepath.Join(staging, asset)
	emit("$ download %s", url)
	if err := downloadVerified(ctx, url, zipPath, wantSHA, emit); err != nil {
		return err
	}

	unpacked := filepath.Join(staging, "unpacked")
	emit("$ extract %s", asset)
	if err := unzipTo(zipPath, unpacked); err != nil {
		return fmt.Errorf("extract %s: %w", asset, err)
	}
	if !isExecFile(filepath.Join(unpacked, "pwsh.exe")) {
		return fmt.Errorf("archive %s contained no pwsh.exe", asset)
	}

	// Replace any leftover half-installed tree, then move the verified one in.
	_ = os.RemoveAll(ManagedPwshDir())
	if err := os.MkdirAll(filepath.Dir(ManagedPwshDir()), 0o755); err != nil {
		return fmt.Errorf("create runtime dir: %w", err)
	}
	if err := os.Rename(unpacked, ManagedPwshDir()); err != nil {
		return fmt.Errorf("install into %s: %w", ManagedPwshDir(), err)
	}
	if !isExecFile(ManagedPwsh()) {
		return fmt.Errorf("install finished but pwsh is missing at %s", ManagedPwsh())
	}

	// Make it usable in this process — and so in the psmux server we spawn —
	// without waiting for a restart.
	prependPATH(ManagedPwshDir())
	emit("PowerShell %s installed at %s", PwshVersion, ManagedPwsh())
	return nil
}

// downloadVerified streams url to dst, hashing as it goes, and fails if the
// digest does not match want. The hash is computed on the bytes actually written
// rather than by re-reading the file, so a disk that lies about a write cannot
// pass the check.
func downloadVerified(ctx context.Context, url, dst, want string, emit func(string, ...any)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: HTTP %s", url, resp.Status)
	}

	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()

	sum := sha256.New()
	pr := &progressReader{r: resp.Body, total: resp.ContentLength, emit: emit}
	if _, err := io.Copy(io.MultiWriter(f, sum), pr); err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	if err := f.Close(); err != nil {
		return err
	}
	if got := hex.EncodeToString(sum.Sum(nil)); !strings.EqualFold(got, want) {
		return fmt.Errorf("checksum mismatch for %s: got %s, expected %s", filepath.Base(dst), got, want)
	}
	emit("checksum ok (sha256 %s)", want)
	return nil
}

// progressReader emits a progress line every 10 MB. Deliberately coarse: the
// UI renders each line into a scrolling log, and a per-chunk update would flood
// the event bus for no extra information over a 106 MB download.
type progressReader struct {
	r        io.Reader
	total    int64
	read     int64
	reported int64
	emit     func(string, ...any)
}

const progressStep = 10 << 20

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.read += int64(n)
	if p.read-p.reported >= progressStep {
		p.reported = p.read
		if p.total > 0 {
			p.emit("  %d/%d MiB", p.read>>20, p.total>>20)
		} else {
			p.emit("  %d MiB", p.read>>20)
		}
	}
	return n, err
}

// unzipTo extracts src into dir, creating it. Entry names are checked against
// dir before anything is written: the archive is a pinned, hash-verified
// PowerShell release, but a zip reader that honours "../" paths is a hazard
// worth closing at the point of use rather than reasoning about upstream.
func unzipTo(src, dir string) error {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer zr.Close()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, f := range zr.File {
		target := filepath.Join(dir, filepath.FromSlash(f.Name)) //nolint:gosec // validated below
		if !underDir(dir, target) {
			return fmt.Errorf("archive entry escapes destination: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := copyZipEntry(f, target); err != nil {
			return err
		}
	}
	return nil
}

// copyZipEntry writes one archive entry to target. Split out so the reader and
// writer are closed per entry rather than at the end of the whole archive — a
// PowerShell zip holds well over a thousand files.
func copyZipEntry(f *zip.File, target string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, rc); err != nil { //nolint:gosec // pinned, hash-verified archive
		return err
	}
	return out.Close()
}

// underDir reports whether target is inside dir.
func underDir(dir, target string) bool {
	rel, err := filepath.Rel(dir, target)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
