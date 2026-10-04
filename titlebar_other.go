//go:build !darwin

package main

// readDoubleClickPref: there is no such setting off macOS; toggling maximise on
// a titlebar double-click is the Windows and Linux convention.
func readDoubleClickPref() string { return "" }
