package live

import "github.com/halalgami/CodingAgentCommander/internal/ledger/parse"

// Agent is one subagent in a session's crew (vision D18). Memory only.
type Agent struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Model       string `json:"model"`
	Background  bool   `json:"background"`
	Depth       int    `json:"depth"`
	State       string `json:"state"`
	Tool        string `json:"tool"`
	Target      string `json:"target"`
	TargetRel   string `json:"targetRel"` // filled on emit (phase 5b)
	TargetPath  string `json:"-"`         // full path; memory only
	StartedMs   int64  `json:"startedMs"`
	EndedMs     int64  `json:"endedMs"`
	Tools       int    `json:"tools"`
	ToolUseID   string `json:"-"`
	LastMs      int64  `json:"-"`
	open        int
	lastKind    parse.Kind
	lastStop    string
}

// AgentMeta is agent-<id>.meta.json. Every field is optional.
type AgentMeta struct {
	AgentType    string `json:"agentType"`
	Description  string `json:"description"`
	ToolUseID    string `json:"toolUseId"`
	SpawnDepth   int    `json:"spawnDepth"`
	RequestShape string `json:"requestShape"`
	Model        string `json:"model"`
}

const (
	AgentWorking  = "working"
	AgentTool     = "tool"
	AgentFinished = "finished"
	AgentFailed   = "failed"
	AgentLost     = "lost"

	agentQuietMs  = 30_000
	agentLostMs   = 10 * 60_000
	agentLingerMs = 60_000
	maxRemembered = 64
)

// agentCall is a lead Agent/Task tool call still waiting for its result.
type agentCall struct{ ToolID, Desc string }

// agentEnd is an end signal for an agent the tracker has not seen yet.
type agentEnd struct {
	ToolID, AgentID string
	Failed          bool
	At              int64
}

func (a Agent) ended() bool {
	return a.State == AgentFinished || a.State == AgentFailed || a.State == AgentLost
}

// ReduceAgent folds one event from the agent's own transcript.
func ReduceAgent(a Agent, e parse.Event) Agent {
	if e.At > 0 && (a.StartedMs == 0 || e.At < a.StartedMs) {
		a.StartedMs = e.At
	}
	if a.ended() && e.At > a.EndedMs {
		a.State, a.EndedMs = AgentWorking, 0 // resumed: it is writing again
	}
	if e.At > a.LastMs {
		a.LastMs = e.At
	}
	switch e.Kind {
	case parse.Assistant:
		if a.Model == "" && e.Model != "" {
			a.Model = e.Model
		}
		a.lastKind, a.lastStop = parse.Assistant, e.Status
	case parse.ToolUse:
		a.open++
		a.Tools++
		a.lastKind = parse.ToolUse
		if !a.ended() {
			a.State, a.Tool, a.Target, a.TargetPath = AgentTool, e.ToolName, e.Target, e.FilePath
		}
	case parse.ToolResult:
		if a.open > 0 {
			a.open--
		}
		a.lastKind = parse.ToolResult
		if !a.ended() && a.open == 0 {
			a.State, a.Tool, a.Target, a.TargetPath = AgentWorking, "", "", ""
		}
	}
	return a
}

func newAgent(id string, meta AgentMeta, calls []agentCall) Agent {
	a := Agent{
		ID: id, Type: parse.Clip(meta.AgentType, 40), Description: parse.Clip(meta.Description, 80),
		Model: parse.Clip(meta.Model, 40), Background: meta.RequestShape == "background",
		Depth: meta.SpawnDepth, State: AgentWorking, ToolUseID: meta.ToolUseID,
	}
	if a.Description == "" && a.ToolUseID != "" {
		for _, c := range calls {
			if c.ToolID == a.ToolUseID {
				a.Description = parse.Clip(c.Desc, 80)
			}
		}
	}
	return a
}

// upsertAgent folds an agent's events into s.Crew, creating it on first
// sight and applying any end signal that arrived before it was seen.
func upsertAgent(s Session, id string, meta AgentMeta, evs []parse.Event) Session {
	crew := append([]Agent(nil), s.Crew...)
	i := -1
	for j := range crew {
		if crew[j].ID == id {
			i = j
			break
		}
	}
	if i < 0 {
		crew = append(crew, newAgent(id, meta, s.calls))
		i = len(crew) - 1
	}
	a := crew[i]
	for _, e := range evs {
		a = ReduceAgent(a, e)
	}
	ends := s.ends[:0:0]
	for _, d := range s.ends {
		hit := (d.ToolID != "" && d.ToolID == a.ToolUseID) || (d.AgentID != "" && d.AgentID == a.ID)
		if hit && !a.ended() && a.LastMs <= d.At {
			a = endOne(a, d.Failed, d.At)
			continue
		}
		if !hit {
			ends = append(ends, d)
		}
	}
	crew[i] = a
	s.Crew, s.ends = crew, ends
	return recount(s)
}

func endOne(a Agent, failed bool, at int64) Agent {
	a.State, a.Tool, a.Target, a.TargetPath, a.EndedMs = AgentFinished, "", "", "", at
	if failed {
		a.State = AgentFailed
	}
	return a
}

// endAgent applies an end signal from the lead's transcript. An agent the
// tracker has not seen yet is remembered and ended when it appears.
func endAgent(s Session, toolID, agentID string, failed bool, at int64) Session {
	found := false
	crew := append([]Agent(nil), s.Crew...)
	for i := range crew {
		if (toolID != "" && crew[i].ToolUseID == toolID) || (agentID != "" && crew[i].ID == agentID) {
			found = true
			if !crew[i].ended() {
				crew[i] = endOne(crew[i], failed, at)
			}
		}
	}
	s.Crew = crew
	if !found && (toolID != "" || agentID != "") {
		s.ends = append(append([]agentEnd(nil), s.ends...), agentEnd{ToolID: toolID, AgentID: agentID, Failed: failed, At: at})
		if len(s.ends) > maxRemembered {
			s.ends = s.ends[len(s.ends)-maxRemembered:]
		}
	}
	return recount(s)
}

func recount(s Session) Session {
	s.Delegating, s.CrewOut = 0, 0
	for _, a := range s.Crew {
		if a.ended() {
			continue
		}
		if a.Background {
			s.CrewOut++
		} else {
			s.Delegating++
		}
	}
	return s
}

// tickCrew applies the time-based rules: quiet-turn fallback, lost, linger.
func tickCrew(s Session, now int64) (Session, bool) {
	if len(s.Crew) == 0 {
		return s, false
	}
	changed := false
	crew := make([]Agent, 0, len(s.Crew))
	for _, a := range s.Crew {
		// A future timestamp (clock skew) would never age out: its clock starts now.
		if a.LastMs > now {
			a.LastMs = now
			changed = true
		}
		if a.EndedMs > now {
			a.EndedMs = now
			changed = true
		}
		switch {
		case a.ended():
			if now-a.EndedMs >= agentLingerMs {
				changed = true
				continue
			}
		case a.LastMs > 0 && now-a.LastMs >= agentLostMs:
			a.State, a.Tool, a.Target, a.TargetPath, a.EndedMs = AgentLost, "", "", "", now
			changed = true
		// Fallback only for background agents (foreground ones end on the
		// lead's result) whose last message closed the turn.
		case a.Background && a.LastMs > 0 && a.open == 0 && a.lastKind == parse.Assistant && a.lastStop == "end_turn" && now-a.LastMs >= agentQuietMs:
			a = endOne(a, false, now)
			changed = true
		}
		crew = append(crew, a)
	}
	if !changed {
		return s, false
	}
	s.Crew = crew
	return recount(s), true
}
