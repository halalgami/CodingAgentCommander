// Package safegit is the only way Commander runs git against a user's repo.
// A cloned repo can carry config that executes code (core.fsmonitor, hooks);
// every call here disables those, is read-only by allow-list, and is time-capped.
package safegit

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/halalgami/CodingAgentCommander/internal/proc"
)

var ErrNotAllowed = errors.New("safegit: subcommand not allowed")

const Timeout = 5 * time.Second

// MaxOutput caps captured stdout. A var so tests can force truncation.
var MaxOutput = 16 << 20

// ErrTruncated is returned (with the captured prefix) when stdout exceeded
// MaxOutput: callers that can use partial output should, and say so.
var ErrTruncated = errors.New("safegit: output truncated")

var allowed = map[string]bool{"rev-parse": true, "rev-list": true, "log": true, "ls-files": true, "cat-file": true}

// Args returns the full argument list for a hardened invocation.
func Args(sub string, args ...string) []string {
	return append([]string{
		"-c", "core.fsmonitor=false",
		"-c", "core.hooksPath=" + os.DevNull,
		"-c", "protocol.allow=never",
		"--no-pager", sub,
	}, args...)
}

// capped keeps at most max bytes. The buffer is a named field, not embedded:
// an embedded bytes.Buffer brings ReadFrom, which io.Copy (used by exec)
// prefers over Write — and that silently bypassed the cap.
type capped struct {
	buf       bytes.Buffer
	max       int
	truncated bool
}

func (c *capped) Write(p []byte) (int, error) {
	room := c.max - c.buf.Len()
	if len(p) > room {
		c.truncated = true
		if room > 0 {
			c.buf.Write(p[:room])
		}
		return len(p), nil
	}
	c.buf.Write(p)
	return len(p), nil
}

func (c *capped) String() string { return c.buf.String() }

// Run executes an allow-listed read-only git subcommand in dir with the
// default timeout and returns its trimmed stdout.
func Run(ctx context.Context, dir, sub string, args ...string) (string, error) {
	return RunTimeout(ctx, Timeout, dir, sub, args...)
}

// RunTimeout is Run with an explicit timeout (scans of big repos need more
// than the default for ls-files/log). Truncated output comes back with
// ErrTruncated.
func RunTimeout(ctx context.Context, timeout time.Duration, dir, sub string, args ...string) (string, error) {
	if !allowed[sub] {
		return "", fmt.Errorf("%w: %q", ErrNotAllowed, sub)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := proc.Hide(exec.CommandContext(ctx, "git", Args(sub, args...)...))
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0")
	out := &capped{max: MaxOutput}
	cmd.Stdout = out
	if err := cmd.Run(); err != nil {
		return "", err
	}
	if out.truncated {
		return strings.TrimSpace(out.String()), ErrTruncated
	}
	return strings.TrimSpace(out.String()), nil
}
