//go:build !darwin && !windows

package main

import (
	"fmt"
	"os/exec"

	"github.com/halalgami/CodingAgentCommander/internal/proc"
)

// nativeNotifier posts through freedesktop's notify-send so a finished-session
// banner still appears off macOS and Windows. It mirrors the darwin and windows
// nativeNotifier types so app.go's construction (`nativeNotifier{}`) stays
// portable — its absence is why `GOOS=linux go build ./...` had never passed,
// and why projectdocs_open_other.go (the xdg-open opener) was compiled by
// nothing at all.
//
// There is no fallback chain like darwin's. notify-send is the only interface
// worth assuming: it ships with every desktop that has a notification daemon,
// and where it is missing — a bare container, a minimal WM, a headless box —
// there is no banner to show in the first place. The single caller drops the
// error (`_ = a.notifier.Notify(...)`), which is what makes that acceptable.
type nativeNotifier struct{}

func (nativeNotifier) Notify(title, body string) error {
	// notify-send takes the summary and body positionally. "--" first, so a
	// title or body beginning with a dash is never parsed as a flag.
	cmd := proc.Hide(exec.Command("notify-send", "--", title, body))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("notify-send: %w", err)
	}
	return nil
}
