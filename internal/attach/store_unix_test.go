//go:build !windows

package attach

import (
	"path/filepath"
	"syscall"
	"testing"
)

func TestCopyInRefusesAFifo(t *testing.T) {
	s := &Store{Dir: filepath.Join(t.TempDir(), "a"), Now: fixedNow}
	fifo := filepath.Join(t.TempDir(), "p.png")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CopyIn(fifo); err == nil || err.Error() != "p.png is not a regular file" {
		t.Errorf("fifo: %v", err)
	}
}
