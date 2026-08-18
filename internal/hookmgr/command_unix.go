//go:build !windows

package hookmgr

import "fmt"

// command emits the POSIX-shell form: Claude Code runs hooks under sh/bash on
// macOS and Linux, so a `>/dev/null 2>&1` redirect and a trailing `#` comment
// are both safe.
func command(port int, token string) string {
	return fmt.Sprintf(
		"curl -s -m 2 -X POST 'http://localhost:%d/notify?token=%s' -H 'Content-Type: application/json' -d @- >/dev/null 2>&1 # %s",
		port, token, Sentinel)
}
