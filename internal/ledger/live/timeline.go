package live

import (
	"regexp"
	"sort"
	"strings"

	"github.com/halalgami/CodingAgentCommander/internal/ledger/parse"
)

// TimelineEvent is one mark on the River. Memory only; Label is display text.
type TimelineEvent struct {
	Session string `json:"session"`
	At      int64  `json:"at"`
	Kind    string `json:"kind"` // read | edit | shell | web | think | agent | wait | other
	Label   string `json:"label"`
	Agent   string `json:"agent,omitempty"`
}

const (
	timelineWindowMs = 10 * 60_000
	maxLeadEvents    = 300
	maxAgentEvents   = 100
)

type timeline struct {
	lead   []TimelineEvent
	agents map[string][]TimelineEvent
}

// kindOf maps a transcript event to a timeline kind; "" = not shown.
func kindOf(e parse.Event) string {
	switch e.Kind {
	case parse.Assistant:
		return "think"
	case parse.ToolUse:
		switch e.ToolName {
		case "Read", "Grep", "Glob", "LS":
			return "read"
		case "Edit", "Write", "MultiEdit", "NotebookEdit":
			return "edit"
		case "Bash":
			return "shell"
		case "WebFetch", "WebSearch":
			return "web"
		case "Agent", "Task":
			return "agent"
		}
		return "other"
	}
	return ""
}

func labelOf(e parse.Event) string {
	if e.Kind != parse.ToolUse {
		return ""
	}
	l := e.ToolName
	if e.Target != "" {
		l += " " + tailPaths(e.Target)
	}
	return parse.Clip(l, 60)
}

var absPath = regexp.MustCompile(`(^|\s)(/[^\s]+)`)

// tailPaths shortens each absolute path deeper than three folders to its last
// two segments ("…/dir/file"), so the clip keeps the file name, not the home folder.
func tailPaths(s string) string {
	return absPath.ReplaceAllStringFunc(s, func(m string) string {
		lead := m[:len(m)-len(strings.TrimLeft(m, " \t"))]
		parts := strings.Split(strings.Trim(strings.TrimSpace(m), "/"), "/")
		if len(parts) <= 3 {
			return m
		}
		return lead + "…/" + strings.Join(parts[len(parts)-2:], "/")
	})
}

// push appends ev to a lane: a tool call replaces the think mark of its own
// assistant line (same timestamp), then the lane is trimmed to the window
// and the cap.
func push(lane []TimelineEvent, ev TimelineEvent, max int) []TimelineEvent {
	if n := len(lane); n > 0 && ev.Kind != "think" && ev.Kind != "wait" && lane[n-1].Kind == "think" && lane[n-1].At == ev.At {
		lane = lane[:n-1]
	}
	lane = append(lane, ev)
	cut := 0
	newest := ev.At
	for cut < len(lane) && newest-lane[cut].At > timelineWindowMs {
		cut++
	}
	if len(lane)-cut > max {
		cut = len(lane) - max
	}
	if cut > 0 {
		lane = append([]TimelineEvent(nil), lane[cut:]...)
	}
	return lane
}

func (tl *timeline) add(ev TimelineEvent) {
	if ev.Agent == "" {
		tl.lead = push(tl.lead, ev, maxLeadEvents)
	} else {
		if tl.agents == nil {
			tl.agents = map[string][]TimelineEvent{}
		}
		tl.agents[ev.Agent] = push(tl.agents[ev.Agent], ev, maxAgentEvents)
	}
	// A finished agent never pushes again, so its lane is only trimmed here:
	// drop every agent lane whose newest mark has left the window.
	for id, lane := range tl.agents {
		if n := len(lane); n == 0 || ev.At-lane[n-1].At > timelineWindowMs {
			delete(tl.agents, id)
		}
	}
}

func (tl *timeline) all() []TimelineEvent {
	out := append([]TimelineEvent(nil), tl.lead...)
	for _, lane := range tl.agents {
		out = append(out, lane...)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At < out[j].At })
	return out
}
