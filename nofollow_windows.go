//go:build windows

package main

import "os"

// openNoFollow on Windows relies on the Lstat guard alone: there is no
// O_NOFOLLOW, and creating a symlink needs privilege there.
func openNoFollow(path string) (*os.File, error) { return os.Open(path) }
