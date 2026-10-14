// Package live holds what each open Claude Code session is doing right now.
// Reduce is pure; Tracker adds locking and a change callback.
package live

import "github.com/halalgami/CodingAgentCommander/internal/ledger/parse"

type State string

const (
	Thinking   State = "thinking"
	Tool       State = "tool"
	Permission State = "permission"
	Finished   State = "finished"
	Idle       State = "idle"
	Error      State = "error"
)

type HookKind string

const (
	HookStop       HookKind = "stop"
	HookPermission HookKind = "permission_prompt"
	HookIdle       HookKind = "idle_prompt"
	HookNeedsInput HookKind = "agent_needs_input"
)

type Session struct {
	ID            string  `json:"id"`
	Cwd           string  `json:"cwd"`
	Branch        string  `json:"branch"`
	Title         string  `json:"title"`
	Summary       string  `json:"summary"`
	State         State   `json:"state"`
	Tool          string  `json:"tool"`
	Target        string  `json:"target"`
	TargetRel     string  `json:"targetRel"`   // Target relative to the project root; filled on emit (phase 5b)
	CompactedMs   int64   `json:"compactedMs"` // last context compaction (phase 5b pose)
	SinceMs       int64   `json:"sinceMs"`
	TurnOpen      bool    `json:"turnOpen"`
	Unread        bool    `json:"unread"`
	External      bool    `json:"external"`
	ContextTokens int64   `json:"contextTokens"`
	Messages      int64   `json:"messages"` // assistant messages; matches the card's existing "turns"
	LastEventMs   int64   `json:"lastEventMs"`
	Model         string  `json:"model"`
	Crew          []Agent `json:"crew"`
	Delegating    int     `json:"delegating"` // foreground agents out: the lead waits on them
	CrewOut       int     `json:"crewOut"`    // background agents out: the session wakes when they report
	Orphan        bool    `json:"orphan"`     // known only from its agents so far; the UI skips it
	calls         []agentCall
	ends          []agentEnd
	Pending       []string `json:"-"`
	TargetPath    string   `json:"-"` // full path of the file the current tool works on; memory only
}

func enter(s Session, st State, tool, target string, at int64) Session {
	if st != Tool {
		s.TargetPath = ""
	}
	if s.State == st && s.Tool == tool && s.Target == target {
		return s
	}
	s.State, s.Tool, s.Target, s.SinceMs = st, tool, target, at
	return s
}

func without(ids []string, id string) []string {
	out := ids[:0:0]
	for _, x := range ids {
		if x != id {
			out = append(out, x)
		}
	}
	return out
}

// Reduce folds one transcript event into a session.
func Reduce(s Session, e parse.Event) Session {
	if s.ID == "" {
		s.ID = e.SessionID
	}
	if e.Cwd != "" {
		s.Cwd = e.Cwd
	}
	if e.Branch != "" {
		s.Branch = e.Branch
	}
	if e.At > s.LastEventMs {
		s.LastEventMs = e.At
	}
	if e.Sidechain {
		return s
	}
	s.Orphan = false
	switch e.Kind {
	case parse.Compact:
		s.CompactedMs = e.At
	case parse.Title:
		s.Title = e.Text
	case parse.AwaySummary:
		s.Summary = e.Text
	case parse.Prompt:
		s.TurnOpen, s.Unread, s.Pending = true, false, nil
		s = enter(s, Thinking, "", "", e.At)
	case parse.Notification:
		// A background task reported back: Claude resumes on its own. Not a
		// prompt, so whatever was unread stays unread.
		s = endAgent(s, e.ToolID, e.AgentID, e.Status != "" && e.Status != "completed", e.At)
		s.TurnOpen = true
		s = enter(s, Thinking, "", "", e.At)
	case parse.Assistant:
		if e.Model != "" {
			s.Model = e.Model
		}
		s.Messages++
		if e.HasUsage {
			s.ContextTokens = e.Usage.In + e.Usage.CacheRead + e.Usage.CacheWrite
		}
		if len(s.Pending) == 0 && s.State != Permission {
			s.TurnOpen = true
			s = enter(s, Thinking, "", "", e.At)
		}
	case parse.ToolUse:
		s.TurnOpen = true
		s.Pending = append(append([]string{}, s.Pending...), e.ToolID)
		s.TargetPath = e.FilePath
		s = enter(s, Tool, e.ToolName, e.Target, e.At)
		if e.ToolName == "Agent" || e.ToolName == "Task" {
			s.calls = append(append([]agentCall(nil), s.calls...), agentCall{ToolID: e.ToolID, Desc: e.Target})
			if len(s.calls) > maxRemembered {
				s.calls = s.calls[len(s.calls)-maxRemembered:]
			}
		}
	case parse.ToolResult:
		for i, c := range s.calls {
			if c.ToolID != e.ToolID {
				continue
			}
			s.calls = append(append([]agentCall(nil), s.calls[:i]...), s.calls[i+1:]...)
			if e.Status != "async_launched" { // a background launch returns at once
				s = endAgent(s, e.ToolID, e.AgentID, e.IsError, e.At)
			}
			break
		}
		s.Pending = without(s.Pending, e.ToolID)
		if len(s.Pending) == 0 {
			s = enter(s, Thinking, "", "", e.At)
		} else if s.State == Permission {
			s = enter(s, Tool, s.Tool, s.Target, e.At)
		}
	case parse.TurnDuration:
		s.TurnOpen, s.Pending = false, nil
		if s.State != Finished && s.State != Idle {
			s.Unread = true
			s = enter(s, Finished, "", "", e.At)
		}
	case parse.APIError:
		s.TurnOpen, s.Pending = false, nil
		s = enter(s, Error, "", "", e.At)
	case parse.Interrupted:
		// Esc: the user is at the keyboard, so nothing is waiting unread.
		s.TurnOpen, s.Pending, s.Unread = false, nil, false
		s = enter(s, Idle, "", "", e.At)
	}
	return s
}

// ReduceHook folds a Claude Code hook notification into a session.
func ReduceHook(s Session, k HookKind, at int64) Session {
	switch k {
	case HookStop:
		s.TurnOpen, s.Pending = false, nil
		if s.State != Finished {
			s.Unread = true
			s = enter(s, Finished, "", "", at)
		}
	case HookPermission, HookNeedsInput:
		// The hook fires ~6 s after the prompt appears; if the tool already
		// returned, the prompt is gone and the hook is stale.
		if len(s.Pending) > 0 {
			s = enter(s, Permission, s.Tool, s.Target, at)
		}
	case HookIdle:
		if !s.TurnOpen && !(s.State == Finished && s.Unread) {
			s = enter(s, Idle, "", "", at)
		}
	}
	return s
}

// Ack records that the user looked at a finished session.
func Ack(s Session, at int64) Session {
	s.Unread = false
	if s.State == Finished {
		s = enter(s, Idle, "", "", at)
	}
	return s
}
