package main

import "os/exec"

// readDoubleClickPref returns the system "double-click a window's title bar to"
// setting. A non-zero exit means the key was never set, i.e. the default (zoom).
func readDoubleClickPref() string {
	out, err := exec.Command("defaults", "read", "-g", "AppleActionOnDoubleClick").Output()
	if err != nil {
		return ""
	}
	return string(out)
}
