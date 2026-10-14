//go:build windows

package store

// lockDir is a no-op on Windows in phase 1: two Commander instances there are
// rare (no stale-bundle problem), and LockFileEx needs x/sys. Documented in the
// ledger spec §4.6.
func lockDir(string) (func(), bool, error) { return func() {}, true, nil }
