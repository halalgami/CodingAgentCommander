//go:build !windows

package main

import (
	"os"
	"syscall"
)

// openNoFollow refuses to open a symlink, closing the gap between the Lstat
// guard and the open.
func openNoFollow(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
}
