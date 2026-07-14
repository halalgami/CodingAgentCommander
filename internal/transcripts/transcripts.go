// Package transcripts reads Claude Code session transcripts from ~/.claude.
package transcripts

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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

// ContextTokens returns the current context size of a transcript: the last
// assistant message's input + cache-read + cache-creation tokens.
func ContextTokens(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open transcript: %w", err)
	}
	defer f.Close()

	last := -1
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1024*1024), 16*1024*1024) // large lines
	for sc.Scan() {
		var l line
		if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
			continue // skip malformed lines
		}
		if l.Type == "assistant" && l.Message.Usage != nil {
			u := l.Message.Usage
			last = u.InputTokens + u.CacheReadInputTokens + u.CacheCreationInputTokens
		}
	}
	if err := sc.Err(); err != nil {
		return 0, err
	}
	if last < 0 {
		return 0, fmt.Errorf("no assistant usage found in %s", path)
	}
	return last, nil
}

// TurnCount returns the number of assistant messages in a transcript.
func TurnCount(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	n := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1024*1024), 16*1024*1024)
	for sc.Scan() {
		var l line
		if json.Unmarshal(sc.Bytes(), &l) != nil {
			continue
		}
		if l.Type == "assistant" {
			n++
		}
	}
	return n, sc.Err()
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
	type f struct {
		path string
		mod  int64
	}
	var files []f
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, f{filepath.Join(dir, e.Name()), info.ModTime().UnixNano()})
	}
	if len(files) == 0 {
		return "", os.ErrNotExist
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mod > files[j].mod })
	return files[0].path, nil
}

// StatsForCwd finds the newest transcript for cwd and returns its context
// tokens, turn count, and path.
func StatsForCwd(projectsRoot, cwd string) (int, int, string, error) {
	newest, err := NewestTranscript(projectsRoot, cwd)
	if err != nil {
		return 0, 0, "", err
	}
	ctx, err := ContextTokens(newest)
	if err != nil {
		return 0, 0, newest, err
	}
	turns, _ := TurnCount(newest)
	return ctx, turns, newest, nil
}
