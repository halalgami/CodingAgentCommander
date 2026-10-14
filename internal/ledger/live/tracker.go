package live

import (
	"sort"
	"sync"
	"time"

	"github.com/halalgami/CodingAgentCommander/internal/ledger/parse"
)

// Tracker is the concurrent home of live sessions. onChange is called outside
// the lock, once per changed session per call.
type Tracker struct {
	mu       sync.Mutex
	sessions map[string]*Session
	onChange func(Session)
	lines    map[string]*timeline
	OnEvent  func([]TimelineEvent)
	// OnRemove is called (outside the lock) with sessions Forget/Expire dropped.
	OnRemove func([]string)
}

func New(onChange func(Session)) *Tracker {
	return &Tracker{sessions: map[string]*Session{}, onChange: onChange, lines: map[string]*timeline{}}
}

// Apply folds events in order. external marks sessions first seen here as not
// launched by Commander.
func (t *Tracker) Apply(evs []parse.Event, external bool) {
	changed := map[string]Session{}
	var tev []TimelineEvent
	t.mu.Lock()
	for _, e := range evs {
		if e.SessionID == "" {
			continue
		}
		cur, ok := t.sessions[e.SessionID]
		if !ok {
			cur = &Session{ID: e.SessionID, External: external}
			t.sessions[e.SessionID] = cur
		}
		prev := *cur
		next := Reduce(prev, e)
		if k := kindOf(e); k != "" && !e.Sidechain {
			t.record(TimelineEvent{Session: e.SessionID, At: e.At, Kind: k, Label: labelOf(e)}, &tev)
		}
		t.recordWait(prev, next, e.At, &tev)
		*cur = next
		changed[e.SessionID] = next
	}
	t.mu.Unlock()
	t.notify(changed)
	t.emit(tev)
}

func (t *Tracker) Hook(id string, k HookKind, at int64) {
	t.update(id, func(s Session) Session { return ReduceHook(s, k, at) })
}

func (t *Tracker) Ack(id string, at int64) {
	t.update(id, func(s Session) Session { return Ack(s, at) })
}

func (t *Tracker) update(id string, f func(Session) Session) {
	t.mu.Lock()
	cur, ok := t.sessions[id]
	if !ok {
		t.mu.Unlock()
		return
	}
	prev := *cur
	*cur = f(prev)
	s := *cur
	var tev []TimelineEvent
	t.recordWait(prev, s, s.SinceMs, &tev)
	t.mu.Unlock()
	t.notify(map[string]Session{id: s})
	t.emit(tev)
}

func (t *Tracker) notify(m map[string]Session) {
	if t.onChange == nil {
		return
	}
	for _, s := range m {
		t.onChange(s)
	}
}

func (t *Tracker) Get(id string) (Session, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	s, ok := t.sessions[id]
	if !ok {
		return Session{}, false
	}
	return *s, true
}

func (t *Tracker) Snapshot() []Session {
	t.mu.Lock()
	out := make([]Session, 0, len(t.sessions))
	for _, s := range t.sessions {
		out = append(out, *s)
	}
	t.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (t *Tracker) Forget(id string) {
	t.mu.Lock()
	_, had := t.sessions[id]
	delete(t.sessions, id)
	delete(t.lines, id)
	t.mu.Unlock()
	if had {
		t.removed([]string{id})
	}
}

func (t *Tracker) removed(ids []string) {
	if t.OnRemove != nil && len(ids) > 0 {
		t.OnRemove(ids)
	}
}

// Expire drops external sessions with no event for longer than idle.
func (t *Tracker) Expire(nowMs int64, idle time.Duration) {
	var gone []string
	t.mu.Lock()
	for id, s := range t.sessions {
		// A lead waiting on its crew is not idle, however long the agents take.
		if s.External && s.Delegating+s.CrewOut == 0 && nowMs-s.LastEventMs > idle.Milliseconds() {
			delete(t.sessions, id)
			delete(t.lines, id)
			gone = append(gone, id)
		}
	}
	t.mu.Unlock()
	t.removed(gone)
}

// ApplyAgent folds a subagent's own transcript events into its session's crew.
func (t *Tracker) ApplyAgent(sessionID, agentID string, meta AgentMeta, evs []parse.Event, external bool) {
	if sessionID == "" || agentID == "" {
		return
	}
	t.mu.Lock()
	cur, ok := t.sessions[sessionID]
	if !ok {
		cur = &Session{ID: sessionID, External: external, Orphan: true}
		t.sessions[sessionID] = cur
	}
	for _, e := range evs {
		if e.At > cur.LastEventMs {
			cur.LastEventMs = e.At
		}
	}
	var tev []TimelineEvent
	for _, e := range evs {
		if k := kindOf(e); k != "" {
			t.record(TimelineEvent{Session: sessionID, At: e.At, Kind: k, Label: labelOf(e), Agent: agentID}, &tev)
		}
	}
	next := upsertAgent(*cur, agentID, meta, evs)
	*cur = next
	t.mu.Unlock()
	t.notify(map[string]Session{sessionID: next})
	t.emit(tev)
}

// Tick applies the crew's time rules (quiet fallback, lost, linger).
func (t *Tracker) Tick(nowMs int64) {
	changed := map[string]Session{}
	t.mu.Lock()
	for id, cur := range t.sessions {
		if next, ok := tickCrew(*cur, nowMs); ok {
			*cur = next
			changed[id] = next
		}
	}
	t.mu.Unlock()
	t.notify(changed)
}

// record adds ev to its session's timeline; caller holds t.mu.
func (t *Tracker) record(ev TimelineEvent, out *[]TimelineEvent) {
	tl := t.lines[ev.Session]
	if tl == nil {
		tl = &timeline{}
		t.lines[ev.Session] = tl
	}
	tl.add(ev)
	*out = append(*out, ev)
}

// recordWait marks entering and leaving a permission wait; caller holds t.mu.
func (t *Tracker) recordWait(prev, next Session, at int64, out *[]TimelineEvent) {
	if prev.State != Permission && next.State == Permission {
		t.record(TimelineEvent{Session: next.ID, At: at, Kind: "wait", Label: "start"}, out)
	} else if prev.State == Permission && next.State != Permission {
		t.record(TimelineEvent{Session: next.ID, At: at, Kind: "wait", Label: "end"}, out)
	}
}

func (t *Tracker) emit(evs []TimelineEvent) {
	if t.OnEvent != nil && len(evs) > 0 {
		t.OnEvent(evs)
	}
}

// Timeline returns each session's events of the last 10 minutes, oldest first.
func (t *Tracker) Timeline() map[string][]TimelineEvent {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make(map[string][]TimelineEvent, len(t.lines))
	for id, tl := range t.lines {
		out[id] = tl.all()
	}
	return out
}
