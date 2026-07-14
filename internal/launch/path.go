package launch

import (
	"os"
	"path/filepath"
	"strings"
)

// AugmentPATH appends the standard macOS CLI install locations to this
// process's PATH. Finder/Dock-launched apps inherit launchd's minimal PATH
// (/usr/bin:/bin:/usr/sbin:/sbin), so tmux, claude, and litellm — installed
// via homebrew, pip --user, or npm — are invisible without this. Children
// (tmux windows running claude) inherit the augmented PATH too.
//
// Sources, in order: existing PATH, homebrew, ~/.local/bin, pip-user bins,
// then every entry in /etc/paths.d/* (where installers register paths for
// login shells). Duplicates are dropped; nothing is ever removed.
func AugmentPATH() {
	sep := string(os.PathListSeparator)
	cur := os.Getenv("PATH")
	seen := map[string]bool{}
	for _, p := range strings.Split(cur, sep) {
		seen[p] = true
	}
	var extra []string
	add := func(p string) {
		if p != "" && !seen[p] {
			seen[p] = true
			extra = append(extra, p)
		}
	}
	home, _ := os.UserHomeDir()
	add("/opt/homebrew/bin")
	add("/opt/homebrew/sbin")
	add("/usr/local/bin")
	if home != "" {
		add(filepath.Join(home, ".local", "bin"))
		if ms, _ := filepath.Glob(filepath.Join(home, "Library", "Python", "*", "bin")); ms != nil {
			for _, m := range ms {
				add(m)
			}
		}
	}
	if entries, err := os.ReadDir("/etc/paths.d"); err == nil {
		for _, e := range entries {
			b, err := os.ReadFile(filepath.Join("/etc/paths.d", e.Name()))
			if err != nil {
				continue
			}
			for _, line := range strings.Split(string(b), "\n") {
				add(strings.TrimSpace(line))
			}
		}
	}
	if len(extra) > 0 {
		os.Setenv("PATH", cur+sep+strings.Join(extra, sep))
	}
}
