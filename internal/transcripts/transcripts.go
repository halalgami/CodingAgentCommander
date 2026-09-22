// Package transcripts reads Claude Code session transcripts from ~/.claude.
package transcripts

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Session summarises one transcript file.
type Session struct {
	ID            string
	ProjectPath   string
	ModTime       time.Time
	ContextTokens int
}

type usage struct {
	InputTokens              int `json:"input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
}

type line struct {
	Type    string `json:"type"`
	Message struct {
		Usage *usage `json:"usage"`
	} `json:"message"`
}

// assistantMarker gates the JSON decode. Most of a transcript by BYTE VOLUME is
// tool results and file contents on user lines, and decoding those costs the
// same as decoding a line we want while yielding nothing — the struct above has
// two fields. The marker is the loose form on purpose: `"type":"assistant"` is
// brittle against whitespace and key order, whereas any line that could satisfy
// Type == "assistant" must contain these bytes somewhere. It is a prefilter, not
// a parser — the decode below still decides.
var assistantMarker = []byte(`"assistant"`)

// Progress is the resumable position of a scan. Transcripts are append-only, so
// a poll that already read the first N bytes never needs to read them again;
// carrying Offset forward turns a per-tick cost proportional to the whole file
// into one proportional to the last turn.
//
// Size is kept alongside Offset to detect the one case where resuming is wrong:
// a file that SHRANK was rewritten rather than appended to, and the bytes behind
// Offset are no longer the bytes we counted.
type Progress struct {
	Size   int64
	Offset int64
	Ctx    int
	Turns  int
	Found  bool // an assistant usage line has been seen at some point
}

// Scan folds a transcript's assistant lines into prev and returns the updated
// progress. A zero prev reads the whole file; a prev from an earlier call reads
// only what was appended since.
//
// One pass yields both statistics. They used to be two exported functions that
// each opened the file, allocated a 1 MB scanner buffer and decoded every line
// in full — for two integers, on every session, every five seconds.
func Scan(path string, prev Progress) (Progress, error) {
	f, err := os.Open(path)
	if err != nil {
		return prev, fmt.Errorf("open transcript: %w", err)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return prev, err
	}
	cur := prev
	cur.Size = fi.Size()
	// Shrunk, or nothing carried over: start clean. Equal size with a live
	// offset means nothing was appended and there is nothing to do.
	if fi.Size() < prev.Size {
		cur = Progress{Size: fi.Size()}
	} else if prev.Offset > 0 && fi.Size() == prev.Size {
		return cur, nil
	}
	if _, err := f.Seek(cur.Offset, io.SeekStart); err != nil {
		return prev, err
	}

	// ReadSlice, not ReadBytes: it returns a view INTO the reader's buffer and
	// allocates nothing, where ReadBytes copies every line out. On the 38 MB
	// transcript here that copy was 66 MB of garbage per scan — more than the
	// two-pass version it replaced, which at least reused a scanner buffer.
	// Lines longer than the buffer come back in pieces (ErrBufferFull) and are
	// stitched into scratch, which is reused across lines.
	br := bufio.NewReaderSize(f, 256*1024)
	var scratch []byte
	for {
		b, readErr := br.ReadSlice('\n')
		if readErr == bufio.ErrBufferFull {
			scratch = append(scratch[:0], b...)
			for readErr == bufio.ErrBufferFull {
				b, readErr = br.ReadSlice('\n')
				scratch = append(scratch, b...)
			}
			b = scratch
		}
		// Only a line terminated by '\n' is complete. A partial tail means the
		// writer is mid-append: leave it unconsumed so the next call re-reads it
		// whole rather than counting half a turn.
		if n := len(b); n > 0 && b[n-1] == '\n' {
			cur.Offset += int64(n)
			if bytes.Contains(b, assistantMarker) {
				var l line
				if json.Unmarshal(b, &l) == nil && l.Type == "assistant" {
					cur.Turns++
					if u := l.Message.Usage; u != nil {
						cur.Ctx = u.InputTokens + u.CacheReadInputTokens + u.CacheCreationInputTokens
						cur.Found = true
					}
				}
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return prev, readErr
		}
	}
	return cur, nil
}

// ContextTokens returns the current context size of a transcript: the last
// assistant message's input + cache-read + cache-creation tokens.
func ContextTokens(path string) (int, error) {
	p, err := Scan(path, Progress{})
	if err != nil {
		return 0, err
	}
	if !p.Found {
		return 0, fmt.Errorf("no assistant usage found in %s", path)
	}
	return p.Ctx, nil
}

// TurnCount returns the number of assistant messages in a transcript.
func TurnCount(path string) (int, error) {
	p, err := Scan(path, Progress{})
	if err != nil {
		return 0, err
	}
	return p.Turns, nil
}

// EncodeCwd mirrors Claude Code's project-dir encoding: '/' and '.' become '-'.
func EncodeCwd(cwd string) string {
	r := strings.NewReplacer("/", "-", ".", "-")
	return r.Replace(cwd)
}

// NewestTranscript returns the path of the most recently modified transcript
// for cwd (no parsing), so callers can cache parsed stats by path+mtime.
func NewestTranscript(projectsRoot, cwd string) (string, error) {
	dir := filepath.Join(projectsRoot, EncodeCwd(cwd))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	// Single-pass max, not a slice plus a sort: this runs per session per poll
	// while a session has no transcript path recorded yet, and a long-lived
	// project accumulates transcripts without bound (54 in the busiest dir here).
	// e.Info() is a lazy lstat per entry, so the loop is already the expensive
	// part — there is no reason to also allocate and sort.
	best, bestMod := "", int64(0)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if mod := info.ModTime().UnixNano(); best == "" || mod > bestMod {
			best, bestMod = filepath.Join(dir, e.Name()), mod
		}
	}
	if best == "" {
		return "", os.ErrNotExist
	}
	return best, nil
}

// StatsForCwd finds the newest transcript for cwd and returns its context
// tokens, turn count, and path.
func StatsForCwd(projectsRoot, cwd string) (int, int, string, error) {
	newest, err := NewestTranscript(projectsRoot, cwd)
	if err != nil {
		return 0, 0, "", err
	}
	p, err := Scan(newest, Progress{})
	if err != nil {
		return 0, 0, newest, err
	}
	if !p.Found {
		return 0, 0, newest, fmt.Errorf("no assistant usage found in %s", newest)
	}
	return p.Ctx, p.Turns, newest, nil
}
