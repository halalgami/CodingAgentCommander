package main

import (
	"context"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/halalgami/CodingAgentCommander/internal/ledger/live"
	"github.com/halalgami/CodingAgentCommander/internal/rooms"
	"github.com/halalgami/CodingAgentCommander/internal/rooms/roomlog"
	"github.com/halalgami/CodingAgentCommander/internal/safegit"
)

// Automatic status (rooms spec §8.1): a pump that watches room sessions and
// posts coarse, coalesced status events. They are data, never prompt text.

const (
	statusTick     = 5 * time.Second
	stateCoalesce  = 30 * time.Second
	gitPollEvery   = 20 * time.Second
	finishSummaryN = 600
)

// winInfo is what the pump needs to know about one Commander window.
type winInfo struct {
	Window, Cwd, Label, Model, ClaudeID, Tag string
	Launched                                 time.Time
}

// liveInfo is the ledger's view of a session.
type liveInfo struct {
	Title string
	State string // coarse: working | idle | needs approval | errored
	Turns int64
}

// gitInfo is a session folder's git state.
type gitInfo struct {
	OK            bool
	Branch, Sha   string
	Dirty, Pushed bool
	Subject       string
}

type roomSess struct {
	room          string
	title, state  string
	pendingState  string
	stateAt       time.Time
	git           gitInfo
	gitAt         time.Time
	listening     string
	listenChanged bool
	key           string // the status session key: the launch tag, else the window id
}

type statusPump struct {
	mu   sync.Mutex
	sess map[string]*roomSess // window id → tracked room session

	now       func() time.Time
	windows   func() []winInfo
	roomOf    func(cwd string) string // open room id, or ""
	live      func(claudeID string) (liveInfo, bool)
	git       func(cwd string, withPushed bool) gitInfo
	post      func(room string, st roomlog.Status) error
	summaries func(room string) bool
	machine   string
	// mine lists this machine's live sessions per room, as peers see them;
	// the first tick ends those that no longer run (crash, restart).
	mine  func() map[string][]string
	swept bool
}

// keyOf: the session's status key. The launch tag survives tmux restarts that
// reuse window ids (@N), so a new session never merges into an old record.
func keyOf(w winInfo) string {
	if w.Tag != "" {
		return w.Tag
	}
	return w.Window
}

func newStatusPump() *statusPump { return &statusPump{sess: map[string]*roomSess{}, now: time.Now} }

func coarse(st live.State) string {
	switch st {
	case live.Thinking, live.Tool:
		return "working"
	case live.Permission:
		return "needs approval"
	case live.Error:
		return "errored"
	}
	return "idle"
}

// tick: start/stop tracking, titles, coalesced state, git commits and pushes.
func (p *statusPump) tick() {
	now := p.now()
	wins := p.windows()
	p.mu.Lock()
	defer p.mu.Unlock()
	present := map[string]bool{}
	for _, w := range wins {
		present[w.Window] = true
		room := p.roomOf(w.Cwd)
		rs := p.sess[w.Window]
		if rs != nil && rs.room != room {
			if rs.room != "" {
				p.post(rs.room, roomlog.Status{Event: roomlog.EvEnded, Session: rs.key, Reason: "room changed"})
			}
			delete(p.sess, w.Window)
			rs = nil
		}
		if room == "" {
			continue
		}
		if rs == nil {
			rs = &roomSess{room: room, key: keyOf(w)}
			p.sess[w.Window] = rs
			rs.git, rs.gitAt = p.git(w.Cwd, false), now
			li, _ := p.live(w.ClaudeID)
			rs.title, rs.state, rs.stateAt = li.Title, li.State, now
			if rs.state == "" {
				rs.state = "idle"
			}
			p.post(room, roomlog.Status{Event: roomlog.EvStarted, Session: rs.key, Label: w.Label, Model: w.Model, Machine: p.machine,
				Branch: rs.git.Branch, Sha: rs.git.Sha, Dirty: rs.git.Dirty, State: rs.state, Title: rs.title, Since: now.UTC().Format(time.RFC3339)})
			continue
		}
		if li, ok := p.live(w.ClaudeID); ok {
			if li.Title != "" && li.Title != rs.title {
				rs.title = li.Title
				p.post(room, roomlog.Status{Event: roomlog.EvTitle, Session: rs.key, Title: li.Title})
			}
			if li.State != "" && li.State != rs.state {
				rs.pendingState = li.State
			} else if li.State == rs.state && !rs.listenChanged {
				rs.pendingState = ""
			}
		}
		if rs.pendingState != "" && now.Sub(rs.stateAt) >= stateCoalesce {
			rs.state, rs.pendingState, rs.stateAt, rs.listenChanged = rs.pendingState, "", now, false
			p.post(room, roomlog.Status{Event: roomlog.EvState, Session: rs.key, State: rs.state, Since: now.UTC().Format(time.RFC3339), Listening: rs.listening})
		}
		if now.Sub(rs.gitAt) >= gitPollEvery {
			rs.gitAt = now
			g := p.git(w.Cwd, !rs.git.Pushed) // pushed? is only asked until it is
			switch {
			case !g.OK:
			case g.Sha != rs.git.Sha && g.Sha != "":
				g = p.git(w.Cwd, true) // subject and pushed? only when HEAD moved
				p.post(room, roomlog.Status{Event: roomlog.EvCommit, Session: rs.key, Branch: g.Branch, Sha: g.Sha, Subject: g.Subject, Pushed: g.Pushed, Dirty: g.Dirty})
				rs.git = g
			case g.Pushed && !rs.git.Pushed:
				p.post(room, roomlog.Status{Event: roomlog.EvPush, Session: rs.key, Branch: g.Branch, Sha: g.Sha})
				rs.git.Pushed = true
			}
		}
	}
	if !p.swept && p.mine != nil {
		p.swept = true
		running := map[string]bool{}
		for _, rs := range p.sess {
			running[rs.room+"|"+rs.key] = true
		}
		for room, keys := range p.mine() {
			for _, k := range keys {
				if !running[room+"|"+k] {
					p.post(room, roomlog.Status{Event: roomlog.EvEnded, Session: k, Reason: "no longer running"})
				}
			}
		}
	}
	for win, rs := range p.sess {
		if !present[win] {
			p.post(rs.room, roomlog.Status{Event: roomlog.EvEnded, Session: rs.key, Reason: "session ended"})
			delete(p.sess, win)
		}
	}
}

var mdImage = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`)

// finishSummary: the last assistant message, images stripped, 600 characters.
func finishSummary(s string) string {
	s = strings.TrimSpace(mdImage.ReplaceAllString(s, ""))
	if r := []rune(s); len(r) > finishSummaryN {
		return string(r[:finishSummaryN]) + "…"
	}
	return s
}

// finished posts session.finished for a window after a Stop hook.
func (p *statusPump) finished(w winInfo, lastMessage string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	rs := p.sess[w.Window]
	if rs == nil {
		return
	}
	li, _ := p.live(w.ClaudeID)
	g := p.git(w.Cwd, true)
	if g.OK {
		rs.git = g
	}
	st := roomlog.Status{Event: roomlog.EvFinished, Session: rs.key, Turns: li.Turns, Branch: g.Branch, Sha: g.Sha, Pushed: g.Pushed, State: "idle"}
	if !w.Launched.IsZero() {
		st.DurationS = int64(p.now().Sub(w.Launched).Seconds())
	}
	if p.summaries == nil || p.summaries(rs.room) {
		st.Summary = finishSummary(lastMessage)
	}
	rs.state, rs.pendingState, rs.stateAt = "idle", "", p.now()
	p.post(rs.room, st)
}

// setListening reports a room_wait opening (thread) or ending ("").
func (p *statusPump) setListening(window, thread string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	rs := p.sess[window]
	if rs == nil || rs.listening == thread {
		return
	}
	rs.listening = thread
	// Coalesced like state changes (≤ 1 per 30 s): a wait loop can't flood the log.
	if rs.pendingState == "" {
		rs.pendingState = rs.state
	}
	rs.listenChanged = true
}

// readGit reads branch, HEAD, dirty; with withPushed also subject + pushed?.
func readGit(cwd string, withPushed bool) gitInfo {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sha, err := safegit.Run(ctx, cwd, "rev-parse", "HEAD")
	if err != nil {
		return gitInfo{}
	}
	g := gitInfo{OK: true, Sha: strings.TrimSpace(sha)}
	b, _ := safegit.Run(ctx, cwd, "rev-parse", "--abbrev-ref", "HEAD")
	g.Branch = strings.TrimSpace(b)
	// safegit allows no `status`: modified tracked files stand in for "dirty".
	if st, err := safegit.Run(ctx, cwd, "ls-files", "-m"); err == nil {
		g.Dirty = strings.TrimSpace(st) != ""
	}
	if withPushed {
		subj, _ := safegit.Run(ctx, cwd, "log", "-1", "--format=%s")
		g.Subject = strings.TrimSpace(subj)
		g.Pushed = shaPushed(ctx, cwd, g.Sha)
	}
	return g
}

// shaPushed: some remote-tracking branch contains sha (nothing of sha's
// history is missing from the remotes).
func shaPushed(ctx context.Context, cwd, sha string) bool {
	if !shaRe.MatchString(sha) {
		return false // never hand git anything but a hash (it is an argument)
	}
	if out, err := safegit.Run(ctx, cwd, "rev-list", "--remotes", "-n1"); err != nil || strings.TrimSpace(out) == "" {
		return false // no remote-tracking branches at all
	}
	out, err := safegit.Run(ctx, cwd, "rev-list", "-n1", sha, "--not", "--remotes") // sha is a checked hash: no leading dash
	return err == nil && strings.TrimSpace(out) == ""
}

// winInfoOf: caller holds a.mu.
func winInfoOf(id string, r *sessionRec) winInfo {
	return winInfo{Window: id, Cwd: r.Cwd, Label: r.Name, Model: r.Model, ClaudeID: r.ClaudeSessionID, Tag: r.Tag, Launched: r.LaunchedAt}
}

// startRoomStatus wires the pump to the app and runs it every statusTick.
func (a *App) startRoomStatus(svc interface {
	PostStatus(string, roomlog.Status) error
}) {
	p := newStatusPump()
	p.machine = deviceName()
	p.windows = func() []winInfo {
		a.mu.Lock()
		defer a.mu.Unlock()
		out := make([]winInfo, 0, len(a.sessions))
		for id, r := range a.sessions {
			out = append(out, winInfoOf(id, r))
		}
		return out
	}
	var (
		cmu    sync.Mutex
		cached []rooms.RoomSummary
		at     time.Time
	)
	list := func() []rooms.RoomSummary { // one rooms listing per tick, not per window
		cmu.Lock()
		defer cmu.Unlock()
		if time.Since(at) > 2*time.Second {
			if s, err := a.rs(); err == nil {
				cached, at = s.Rooms(), time.Now()
			}
		}
		return cached
	}
	p.roomOf = func(cwd string) string {
		l := list()
		if len(l) == 0 || cwd == "" {
			return ""
		}
		if r, ok := roomFor(l, resolveProject(cwd)); ok {
			return r.ID
		}
		return ""
	}
	p.mine = func() map[string][]string {
		out := map[string][]string{}
		s, err := a.rs()
		if err != nil {
			return out
		}
		for _, r := range s.Rooms() {
			if r.Closed {
				continue
			}
			if v, err := s.View(r.ID); err == nil {
				for _, l := range v.Live {
					if l.Mine {
						out[r.ID] = append(out[r.ID], l.Session)
					}
				}
			}
		}
		return out
	}
	p.live = func(id string) (liveInfo, bool) {
		if a.ledger == nil || a.ledger.tracker == nil || id == "" {
			return liveInfo{}, false
		}
		s, ok := a.ledger.tracker.Get(id)
		if !ok {
			return liveInfo{}, false
		}
		return liveInfo{Title: s.Title, State: coarse(s.State), Turns: s.Messages}, true
	}
	p.git = readGit
	p.post = svc.PostStatus
	p.summaries = func(room string) bool {
		s, err := a.rs()
		if err != nil {
			return false
		}
		v, err := s.Settings(room)
		return err == nil && v.Summaries
	}
	a.roomStatus.Store(p)
	stop := a.roomsSvc.stop
	go func() {
		t := time.NewTicker(statusTick)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				p.tick()
			}
		}
	}()
}

// openRoomOf: the open room of the project at cwd, or "".
func (a *App) openRoomOf(cwd string) string {
	s, err := a.rs()
	if err != nil || cwd == "" {
		return ""
	}
	list := s.Rooms()
	if len(list) == 0 {
		return ""
	}
	if r, ok := roomFor(list, resolveProject(cwd)); ok {
		return r.ID
	}
	return ""
}
