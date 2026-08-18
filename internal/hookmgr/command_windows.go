//go:build windows

package hookmgr

import "fmt"

// command emits a form that runs identically under both shells Claude Code may
// pick on Windows. Claude Code prefers Git Bash and falls back to PowerShell
// when Git Bash isn't installed, and the hook is written once at startup — we
// can't know which will run it, so the command must satisfy both:
//
//   - No `>/dev/null 2>&1`. PowerShell has no /dev/null and would fail the
//     redirect; `-o NUL` makes curl itself open the Windows null device, which
//     is shell-independent (curl.exe resolves NUL via the Win32 API, so Git
//     Bash does not create a stray file named NUL either).
//   - `-d "@-"` must be quoted. Bare `@-` is a parse error in PowerShell, where
//     a leading `@` begins splatting/array syntax.
//   - Double quotes around the URL and header. Single quotes are literal-string
//     quotes in PowerShell but would not be stripped the same way by every
//     shell; double quotes work in both.
//   - The trailing `#` sentinel comment is a line comment in bash and
//     PowerShell alike, so it stays identifiable without breaking either.
//
// curl.exe has shipped in Windows since 1803, so it needs no install.
func command(port int, token string) string {
	return fmt.Sprintf(
		`curl.exe -s -m 2 -o NUL -X POST "http://localhost:%d/notify?token=%s" -H "Content-Type: application/json" -d "@-" # %s`,
		port, token, Sentinel)
}
