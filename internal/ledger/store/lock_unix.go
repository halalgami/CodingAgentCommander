//go:build !windows

package store

import (
	"os"
	"path/filepath"
	"syscall"
)

// lockDir takes an exclusive, non-blocking flock on dir/LOCK. ok=false means
// another Commander holds it: open read-only.
func lockDir(dir string) (unlock func(), ok bool, err error) {
	f, err := os.OpenFile(filepath.Join(dir, "LOCK"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, false, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return func() {}, false, nil
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, true, nil
}
