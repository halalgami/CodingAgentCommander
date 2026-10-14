//go:build !darwin

package main

// No grabber wired on Windows/Linux in v1 (decision D6): nil hides the actions.
var captureRegion func(dest string) error

var screenshotDir func() string

var isScreenCapture = func(string) bool { return false }
