// Package store is the permanent, aggregate-only project history. It never
// holds prompts, responses, commands, tool targets or file contents.
package store

import (
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/halalgami/CodingAgentCommander/internal/ledger/parse"
	"github.com/halalgami/CodingAgentCommander/internal/metrics/scan"
)

type TokenCounts struct {
	In         int64 `json:"in"`
	Out        int64 `json:"out"`
	CacheRead  int64 `json:"cacheRead"`
	CacheWrite int64 `json:"cacheWrite"`
}

func (t *TokenCounts) add(u parse.Usage) {
	t.In += u.In
	t.Out += u.Out
	t.CacheRead += u.CacheRead
	t.CacheWrite += u.CacheWrite
}

func (t *TokenCounts) merge(o TokenCounts) {
	t.In += o.In
	t.Out += o.Out
	t.CacheRead += o.CacheRead
	t.CacheWrite += o.CacheWrite
}

type Counters struct {
	ActiveMs     int64            `json:"activeMs"`
	Turns        int64            `json:"turns"`
	Sessions     int64            `json:"sessions,omitempty"`
	Tools        map[string]int64 `json:"tools,omitempty"`
	Errors       int64            `json:"errors"`
	Compactions  int64            `json:"compactions"`
	Commits      int64            `json:"commits"`
	TestRuns     int64            `json:"testRuns"`
	TestFails    int64            `json:"testFails"`
	LinesAdded   int64            `json:"linesAdded"`
	LinesRemoved int64            `json:"linesRemoved"`
	Subagents    int64            `json:"subagents"`
	Skipped      int64            `json:"skipped"`
	Tokens       TokenCounts      `json:"tokens"`
}

func (c *Counters) merge(o Counters) {
	c.ActiveMs += o.ActiveMs
	c.Turns += o.Turns
	c.Sessions += o.Sessions
	c.Errors += o.Errors
	c.Compactions += o.Compactions
	c.Commits += o.Commits
	c.TestRuns += o.TestRuns
	c.TestFails += o.TestFails
	c.LinesAdded += o.LinesAdded
	c.LinesRemoved += o.LinesRemoved
	c.Subagents += o.Subagents
	c.Skipped += o.Skipped
	c.Tokens.merge(o.Tokens)
	for k, v := range o.Tools {
		if c.Tools == nil {
			c.Tools = map[string]int64{}
		}
		c.Tools[k] += v
	}
}

type SessionSummary struct {
	ID            string                 `json:"id"`
	Title         string                 `json:"title,omitempty"`
	Branch        string                 `json:"branch,omitempty"`
	Entrypoint    string                 `json:"entrypoint,omitempty"`
	StartedAt     int64                  `json:"startedAt"`
	EndedAt       int64                  `json:"endedAt"`
	Models        map[string]int64       `json:"models,omitempty"`
	TokensByModel map[string]TokenCounts `json:"tokensByModel,omitempty"`
	Days          map[string]Counters    `json:"days,omitempty"`
	Counters
	ParseStatus   string   `json:"parseStatus"`
	ParserVersion int      `json:"parserVersion"`
	PendingTests  []string `json:"pendingTests,omitempty"`
	// LastMsgID/LastUsage: Claude Code writes one assistant message as several
	// lines that repeat its id and usage; only the change since the previous
	// line of the same message is counted.
	LastMsgID string      `json:"lastMsgID,omitempty"`
	LastUsage parse.Usage `json:"lastUsage,omitempty"`
	// Progress counters (parser v3).
	MaxContext      int64 `json:"maxContext,omitempty"`
	NightTurns      int64 `json:"nightTurns,omitempty"`
	CleanStreak     int64 `json:"cleanStreak,omitempty"`
	BestCleanStreak int64 `json:"bestCleanStreak,omitempty"`
	FastCommits     int64 `json:"fastCommits,omitempty"`
	LastPromptAt    int64 `json:"lastPromptAt,omitempty"`
}

// FileTouch is heat, not an exact count: re-deriving a session after a reset
// can add its touches again.
type FileTouch struct {
	Reads  int64 `json:"reads"`
	Edits  int64 `json:"edits"`
	Writes int64 `json:"writes"`
	LastAt int64 `json:"lastAt"`
}

// Cursor lives inside the project file so cursor and aggregates are written
// atomically together: a crash can never leave one ahead of the other.
type Cursor struct {
	Size          int64  `json:"size"`
	Offset        int64  `json:"offset"`
	Mtime         int64  `json:"mtime"`
	SessionID     string `json:"sessionID"`
	Cwd           string `json:"cwd,omitempty"`
	Subagent      bool   `json:"subagent,omitempty"`
	ParserVersion int    `json:"parserVersion"`
}

type Project struct {
	Key         string                      `json:"key"`
	Identity    string                      `json:"identity"`
	Root        string                      `json:"root"`
	Label       string                      `json:"label"`
	FirstSeen   int64                       `json:"firstSeen"`
	LastSeen    int64                       `json:"lastSeen"`
	Missing     bool                        `json:"missing"`
	Plot        int                         `json:"plot,omitempty"` // 1-based city plot (dashboard phase 5a); 0 = unassigned
	Totals      Counters                    `json:"totals"`
	Days        map[string]Counters         `json:"days,omitempty"`
	Files       map[string]FileTouch        `json:"files,omitempty"`
	Sessions    []SessionSummary            `json:"sessions"`
	SeenTurns   map[string]string           `json:"seenTurns,omitempty"` // turn uuid → owning session
	Cursors     map[string]Cursor           `json:"cursors,omitempty"`   // transcript path → read position
	Metrics     map[string]scan.FileMetrics `json:"metrics,omitempty"`
	Snapshots   []scan.Snapshot             `json:"snapshots,omitempty"`
	ScannedAt   int64                       `json:"scannedAt,omitempty"`
	ScanHead    string                      `json:"scanHead,omitempty"`
	ScanPartial bool                        `json:"scanPartial,omitempty"`
	Unlocks     []Unlock                    `json:"unlocks,omitempty"`
}

var (
	commitRe = regexp.MustCompile(`\bgit\s+(-\S+\s+)*commit\b`)
	testRe   = regexp.MustCompile(`\b(go\s+test|vitest|jest|pytest|cargo\s+test|(npm|pnpm|yarn)\s+(run\s+)?test|playwright\s+test)\b`)
)

func (p *Project) session(id string) *SessionSummary {
	for i := range p.Sessions {
		if p.Sessions[i].ID == id {
			return &p.Sessions[i]
		}
	}
	p.Sessions = append(p.Sessions, SessionSummary{ID: id, ParseStatus: "ok", ParserVersion: parse.ParserVersion})
	return &p.Sessions[len(p.Sessions)-1]
}

func removeID(ids *[]string, id string) bool {
	for i, x := range *ids {
		if x == id {
			*ids = append((*ids)[:i], (*ids)[i+1:]...)
			return true
		}
	}
	return false
}

func (p *Project) fold(s *SessionSummary, e parse.Event, subagent, storeTitles bool) {
	day := ""
	if e.At > 0 {
		day = time.UnixMilli(e.At).Format("2006-01-02")
		if s.StartedAt == 0 || e.At < s.StartedAt {
			s.StartedAt = e.At
		}
		if e.At > s.EndedAt {
			s.EndedAt = e.At
		}
	}
	both := func(f func(c *Counters)) {
		f(&s.Counters)
		if day != "" {
			if s.Days == nil {
				s.Days = map[string]Counters{}
			}
			c := s.Days[day]
			f(&c)
			s.Days[day] = c
		}
	}
	if e.Partial && s.ParseStatus == "ok" {
		s.ParseStatus = "partial"
	}
	if !subagent {
		if e.Branch != "" {
			s.Branch = e.Branch
		}
		if e.Entrypoint != "" {
			s.Entrypoint = e.Entrypoint
		}
	}
	switch e.Kind {
	case parse.Prompt:
		if !subagent && e.At > 0 {
			s.LastPromptAt = e.At
		}
	case parse.Title:
		if storeTitles && !subagent {
			s.Title = e.Text
		}
	case parse.Assistant:
		if e.HasUsage {
			if c := e.Usage.In + e.Usage.CacheRead + e.Usage.CacheWrite; c > s.MaxContext {
				s.MaxContext = c
			}
		}
		same := e.MessageID != "" && e.MessageID == s.LastMsgID
		u := e.Usage
		if same {
			u = parse.Usage{In: u.In - s.LastUsage.In, Out: u.Out - s.LastUsage.Out,
				CacheRead: u.CacheRead - s.LastUsage.CacheRead, CacheWrite: u.CacheWrite - s.LastUsage.CacheWrite}
		}
		if e.HasUsage {
			s.LastUsage = e.Usage
		}
		s.LastMsgID = e.MessageID
		if e.Model != "" && e.Model != "<synthetic>" {
			if s.Models == nil {
				s.Models = map[string]int64{}
				s.TokensByModel = map[string]TokenCounts{}
			}
			if !same {
				s.Models[e.Model]++
			}
			if e.HasUsage {
				tb := s.TokensByModel[e.Model]
				tb.add(u)
				s.TokensByModel[e.Model] = tb
			}
		}
		if e.HasUsage {
			both(func(c *Counters) { c.Tokens.add(u) })
		}
	case parse.ToolUse:
		isTest := e.ToolName == "Bash" && testRe.MatchString(e.Command)
		both(func(c *Counters) {
			if c.Tools == nil {
				c.Tools = map[string]int64{}
			}
			c.Tools[e.ToolName]++
			c.LinesAdded += e.LinesAdded
			c.LinesRemoved += e.LinesRemoved
			if e.ToolName == "Task" || e.ToolName == "Agent" {
				c.Subagents++
			}
			if e.ToolName == "Bash" && commitRe.MatchString(e.Command) {
				c.Commits++
			}
			if isTest {
				c.TestRuns++
			}
		})
		// Not for subagents: their lines fold in file-read order, not by time.
		if !subagent && e.ToolName == "Bash" && commitRe.MatchString(e.Command) && s.LastPromptAt > 0 && e.At >= s.LastPromptAt && e.At-s.LastPromptAt <= 60000 {
			s.FastCommits++
		}
		if isTest {
			s.PendingTests = append(s.PendingTests, e.ToolID)
		}
		p.touch(e)
	case parse.ToolResult:
		wasTest := removeID(&s.PendingTests, e.ToolID)
		if e.IsError {
			s.CleanStreak = 0
			both(func(c *Counters) {
				c.Errors++
				if wasTest {
					c.TestFails++
				}
			})
		}
	case parse.TurnDuration:
		if subagent {
			return
		}
		if e.UUID != "" {
			if owner, ok := p.SeenTurns[e.UUID]; ok && owner != s.ID {
				return // history copied into a fork; already counted
			}
			if p.SeenTurns == nil {
				p.SeenTurns = map[string]string{}
			}
			p.SeenTurns[e.UUID] = s.ID
		}
		both(func(c *Counters) { c.ActiveMs += e.Ms; c.Turns++ })
		if e.At > 0 && time.UnixMilli(e.At).Hour() < 4 {
			s.NightTurns++
		}
		s.CleanStreak++
		if s.CleanStreak > s.BestCleanStreak {
			s.BestCleanStreak = s.CleanStreak
		}
	case parse.Compact:
		if !subagent {
			both(func(c *Counters) { c.Compactions++ })
		}
	case parse.APIError:
		s.CleanStreak = 0
		both(func(c *Counters) { c.Errors++ })
	}
}

func (p *Project) touch(e parse.Event) {
	if e.FilePath == "" || p.Root == "" {
		return
	}
	rel, err := filepath.Rel(p.Root, e.FilePath)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return // outside the repo: not part of this city
	}
	rel = filepath.ToSlash(rel)
	if p.Files == nil {
		p.Files = map[string]FileTouch{}
	}
	f := p.Files[rel]
	switch e.ToolName {
	case "Read":
		f.Reads++
	case "Write":
		f.Writes++
	default:
		f.Edits++
	}
	if e.At > f.LastAt {
		f.LastAt = e.At
	}
	p.Files[rel] = f
}

// recompute derives totals and day buckets from the sessions. Never add to
// totals directly: deriving them makes ingest idempotent.
func (p *Project) recompute() {
	p.Totals = Counters{}
	p.Days = map[string]Counters{}
	p.FirstSeen, p.LastSeen = 0, 0
	for _, s := range p.Sessions {
		p.Totals.merge(s.Counters)
		for d, c := range s.Days {
			x := p.Days[d]
			x.merge(c)
			p.Days[d] = x
		}
		if s.StartedAt > 0 && (p.FirstSeen == 0 || s.StartedAt < p.FirstSeen) {
			p.FirstSeen = s.StartedAt
		}
		if s.EndedAt > p.LastSeen {
			p.LastSeen = s.EndedAt
		}
	}
	p.Totals.Sessions = int64(len(p.Sessions))
}

// resetSession forgets a session so its transcript can be re-derived from zero.
func (p *Project) resetSession(id string) {
	kept := p.Sessions[:0]
	for _, s := range p.Sessions {
		if s.ID != id {
			kept = append(kept, s)
		}
	}
	p.Sessions = kept
	for u, owner := range p.SeenTurns {
		if owner == id {
			delete(p.SeenTurns, u)
		}
	}
	for path, c := range p.Cursors {
		if c.SessionID == id {
			delete(p.Cursors, path)
		}
	}
	p.recompute()
}
