package parse

import (
	"bytes"
	"encoding/json"
	"net/url"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxTitle   = 120
	maxSummary = 300
	maxTarget  = 60
	maxCommand = 2048
)

var markers = [][]byte{
	[]byte(`"type":"assistant"`),
	[]byte(`"type":"user"`),
	[]byte(`"turn_duration"`),
	[]byte(`"ai-title"`),
	[]byte(`"away_summary"`),
	[]byte(`"compact_boundary"`),
}

type usage struct {
	Input      int64 `json:"input_tokens"`
	Output     int64 `json:"output_tokens"`
	CacheRead  int64 `json:"cache_read_input_tokens"`
	CacheWrite int64 `json:"cache_creation_input_tokens"`
}

type message struct {
	ID      string          `json:"id"`
	Model   string          `json:"model"`
	Content json.RawMessage `json:"content"`
	Usage   *usage          `json:"usage"`
	Stop    string          `json:"stop_reason"`
}

type block struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	IsError   bool            `json:"is_error"`
}

type record struct {
	Type             string          `json:"type"`
	Subtype          string          `json:"subtype"`
	Timestamp        string          `json:"timestamp"`
	SessionID        string          `json:"sessionId"`
	Cwd              string          `json:"cwd"`
	GitBranch        string          `json:"gitBranch"`
	Entrypoint       string          `json:"entrypoint"`
	UUID             string          `json:"uuid"`
	IsSidechain      bool            `json:"isSidechain"`
	IsMeta           bool            `json:"isMeta"`
	IsCompactSummary bool            `json:"isCompactSummary"`
	IsAPIError       bool            `json:"isApiErrorMessage"`
	DurationMs       *int64          `json:"durationMs"`
	AITitle          string          `json:"aiTitle"`
	Content          json.RawMessage `json:"content"`
	CompactMetadata  *struct {
		PreTokens  int64 `json:"preTokens"`
		PostTokens int64 `json:"postTokens"`
	} `json:"compactMetadata"`
	Message *message `json:"message"`
	Origin  *struct {
		Kind string `json:"kind"`
	} `json:"origin"`
	ToolUseResult json.RawMessage `json:"toolUseResult"`
}

// Classify returns the events in one transcript line (possibly several: an
// assistant message with N tool calls yields 1+N). Unknown or malformed lines
// yield nil.
func Classify(line []byte) []Event {
	if len(line) == 0 || len(line) > MaxLine || !interesting(line) {
		return nil
	}
	var r record
	if json.Unmarshal(line, &r) != nil {
		return nil
	}
	base := Event{
		SessionID: r.SessionID, Cwd: r.Cwd, Branch: r.GitBranch, Entrypoint: r.Entrypoint,
		UUID: r.UUID, Sidechain: r.IsSidechain, At: millis(r.Timestamp),
	}
	switch r.Type {
	case "ai-title":
		if t := Clip(oneLine(r.AITitle), maxTitle); t != "" {
			e := base
			e.Kind, e.Text = Title, t
			return []Event{e}
		}
	case "system":
		return system(base, r)
	case "assistant":
		return assistant(base, r)
	case "user":
		return user(base, r)
	}
	return nil
}

func system(base Event, r record) []Event {
	e := base
	switch r.Subtype {
	case "turn_duration":
		e.Kind = TurnDuration
		if r.DurationMs == nil {
			e.Partial = true
		} else {
			e.Ms = *r.DurationMs
		}
	case "away_summary":
		var s string
		if json.Unmarshal(r.Content, &s) != nil || strings.TrimSpace(s) == "" {
			return nil
		}
		e.Kind, e.Text = AwaySummary, Clip(oneLine(s), maxSummary)
	case "compact_boundary":
		e.Kind = Compact
		if r.CompactMetadata == nil {
			e.Partial = true
		} else {
			e.PreTokens, e.PostTokens = r.CompactMetadata.PreTokens, r.CompactMetadata.PostTokens
		}
	default:
		return nil
	}
	if e.At == 0 {
		e.Partial = true
	}
	return []Event{e}
}

func assistant(base Event, r record) []Event {
	a := base
	a.Kind = Assistant
	if r.Message == nil {
		a.Partial = true
		return []Event{a}
	}
	a.Model, a.MessageID = r.Message.Model, r.Message.ID
	a.Status = Clip(r.Message.Stop, 32) // stop_reason: "end_turn" closes a turn
	if u := r.Message.Usage; u != nil {
		a.HasUsage = true
		a.Usage = Usage{In: u.Input, Out: u.Output, CacheRead: u.CacheRead, CacheWrite: u.CacheWrite}
	}
	out := []Event{a}
	var blocks []block
	if json.Unmarshal(r.Message.Content, &blocks) == nil {
		for _, b := range blocks {
			if b.Type != "tool_use" {
				continue
			}
			e := base
			e.Kind, e.ToolID, e.ToolName = ToolUse, b.ID, b.Name
			describe(&e, b.Input)
			out = append(out, e)
		}
	}
	if r.IsAPIError {
		e := base
		e.Kind = APIError
		out = append(out, e)
	}
	return out
}

func user(base Event, r record) []Event {
	if r.IsMeta || r.IsCompactSummary || r.Message == nil {
		return nil
	}
	c := bytes.TrimSpace(r.Message.Content)
	if len(c) == 0 {
		return nil
	}
	head := c
	if len(head) > 64 {
		head = head[:64]
	}
	if (r.Origin != nil && r.Origin.Kind == "task-notification") || bytes.Contains(head, []byte("<task-notification>")) {
		return notification(base, c)
	}
	if c[0] == '"' {
		var text string
		_ = json.Unmarshal(c, &text)
		e := base
		e.Kind = Prompt
		if isInterrupt(text) {
			e.Kind = Interrupted
		}
		return []Event{e}
	}
	var blocks []block
	if json.Unmarshal(c, &blocks) != nil {
		return nil
	}
	var out []Event
	prompt, interrupt := false, false
	for _, b := range blocks {
		switch b.Type {
		case "tool_result":
			e := base
			e.Kind, e.ToolID, e.IsError = ToolResult, b.ToolUseID, b.IsError
			out = append(out, e)
		case "text", "image":
			prompt = true
			interrupt = interrupt || isInterrupt(b.Text)
		}
	}
	if len(out) == 0 && prompt {
		e := base
		e.Kind = Prompt
		if interrupt {
			e.Kind = Interrupted
		}
		out = append(out, e)
	}
	if len(r.ToolUseResult) > 0 && r.ToolUseResult[0] == '{' {
		var tr struct {
			Status  string `json:"status"`
			AgentID string `json:"agentId"`
		}
		if json.Unmarshal(r.ToolUseResult, &tr) == nil {
			for i := range out {
				if out[i].Kind == ToolResult {
					out[i].Status, out[i].AgentID = Clip(tr.Status, 32), Clip(tr.AgentID, 64)
				}
			}
		}
	}
	return out
}

// notification reads a task-notification user line: which background task
// reported, for which tool call, and how it ended. It is not a prompt.
func notification(base Event, c json.RawMessage) []Event {
	var text string
	if json.Unmarshal(c, &text) != nil {
		var blocks []block
		if json.Unmarshal(c, &blocks) == nil {
			for _, b := range blocks {
				if b.Type == "text" {
					text += b.Text
				}
			}
		}
	}
	e := base
	e.Kind = Notification
	e.AgentID = Clip(tag(text, "task-id"), 64)
	e.ToolID = Clip(tag(text, "tool-use-id"), 64)
	e.Status = Clip(tag(text, "status"), 32)
	// No task id is a notification that is not from an agent (a background
	// review, say): a valid line, so it does not mark history partial.
	return []Event{e}
}

// tag returns the trimmed text between <name> and </name>, or "".
func tag(s, name string) string {
	open, end := "<"+name+">", "</"+name+">"
	i := strings.Index(s, open)
	if i < 0 {
		return ""
	}
	s = s[i+len(open):]
	j := strings.Index(s, end)
	if j < 0 {
		return ""
	}
	return strings.TrimSpace(s[:j])
}

type toolInput struct {
	FilePath     string `json:"file_path"`
	NotebookPath string `json:"notebook_path"`
	Command      string `json:"command"`
	Pattern      string `json:"pattern"`
	URL          string `json:"url"`
	Query        string `json:"query"`
	Description  string `json:"description"`
	OldString    string `json:"old_string"`
	NewString    string `json:"new_string"`
	Content      string `json:"content"`
	Edits        []struct {
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
	} `json:"edits"`
}

func describe(e *Event, raw json.RawMessage) {
	var in toolInput
	if json.Unmarshal(raw, &in) != nil {
		e.Partial = true
		return
	}
	switch e.ToolName {
	case "Read", "Edit", "MultiEdit", "Write", "NotebookEdit":
		e.FilePath = in.FilePath
		if e.FilePath == "" {
			e.FilePath = in.NotebookPath
		}
		if e.FilePath != "" {
			e.Target = filepath.Base(e.FilePath)
		}
	case "Bash":
		e.Command = Clip(in.Command, maxCommand)
		e.Target = in.Command
	case "Grep", "Glob":
		e.Target = in.Pattern
	case "WebFetch":
		if u, err := url.Parse(in.URL); err == nil {
			e.Target = u.Host
		}
	case "WebSearch":
		e.Target = in.Query
	case "Task", "Agent":
		e.Target = in.Description
	}
	e.Target = Clip(oneLine(e.Target), maxTarget)
	switch e.ToolName {
	case "Edit":
		e.LinesAdded, e.LinesRemoved = lineCount(in.NewString), lineCount(in.OldString)
	case "MultiEdit":
		for _, ed := range in.Edits {
			e.LinesAdded += lineCount(ed.NewString)
			e.LinesRemoved += lineCount(ed.OldString)
		}
	case "Write":
		e.LinesAdded = lineCount(in.Content)
	}
}

func interesting(line []byte) bool {
	for _, m := range markers {
		if bytes.Contains(line, m) {
			return true
		}
	}
	return false
}

func millis(ts string) int64 {
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return 0
	}
	return t.UnixMilli()
}

func lineCount(s string) int64 {
	if s == "" {
		return 0
	}
	n := int64(strings.Count(s, "\n"))
	if !strings.HasSuffix(s, "\n") {
		n++
	}
	return n
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// Clip trims s and cuts it to at most n runes (ending in "…" when cut). Always
// returns valid UTF-8: invalid bytes in s are replaced first.
func Clip(s string, n int) string {
	s = strings.ToValidUTF8(strings.TrimSpace(s), "�")
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
}

// isInterrupt recognises the user line Claude Code writes when Esc stops a
// turn. It is not a prompt: the turn is over and Claude is waiting.
func isInterrupt(text string) bool {
	return strings.HasPrefix(strings.TrimSpace(text), "[Request interrupted by user")
}
