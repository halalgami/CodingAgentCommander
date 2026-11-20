package rooms

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/halalgami/CodingAgentCommander/internal/rooms/roomlog"
)

// RoomSettings are this machine's settings for one room (plan R4).
type RoomSettings struct {
	NoSummaries bool `json:"noSummaries,omitempty"` // finish summaries are on unless turned off (§8.1)
	Budget      int  `json:"budget,omitempty"`      // automatic deliveries per thread (§10.4), 0 = 5
	Rate        int  `json:"rate,omitempty"`        // agent posts per hour, 0 = 30
}

const (
	defaultBudget = 5
	defaultRate   = 30
	maxDrafts     = 50
)

func (rs RoomSettings) budget() int {
	if rs.Budget <= 0 {
		return defaultBudget
	}
	return rs.Budget
}

func (rs RoomSettings) rate() int {
	if rs.Rate <= 0 {
		return defaultRate
	}
	return rs.Rate
}

// SettingsView is RoomSettings with defaults filled in.
type SettingsView struct {
	Summaries bool `json:"summaries"`
	Budget    int  `json:"budget"`
	Rate      int  `json:"rate"`
}

// Draft is an agent post held back by the rate limit (§10.4).
type Draft struct {
	ID       string               `json:"id"`
	Kind     string               `json:"kind"`
	Text     string               `json:"text"`
	Parent   string               `json:"parent,omitempty"`
	Fields   map[string]string    `json:"fields,omitempty"`
	Session  string               `json:"session,omitempty"`
	At       int64                `json:"at"`
	Attach   []roomlog.Attachment `json:"attach,omitempty"`
	Mentions []string             `json:"mentions,omitempty"`
}

// Incoming is a message another device wrote (OnMessage, inbox actions).
type Incoming struct {
	Room     string               `json:"room"`
	RoomName string               `json:"roomName"`
	Ref      string               `json:"ref"`
	Thread   string               `json:"thread"`
	Device   string               `json:"device"`
	Author   string               `json:"author"` // machine name
	Own      bool                 `json:"own"`    // written by one of my own machines
	Kind     string               `json:"kind"`
	Text     string               `json:"text"`
	Fields   map[string]string    `json:"fields,omitempty"`
	Time     string               `json:"time"`
	User     string               `json:"user,omitempty"` // author's display name
	Attach   []roomlog.Attachment `json:"attach,omitempty"`
	// MentionsThis: this machine is mentioned.
	MentionsThis bool `json:"mentionsThis,omitempty"`
}

// Wrap is how room text reaches a session (§10.4): labelled with who wrote
// it (one of my machines or a colleague's), names cleaned, every body line
// quoted so a message can't fake a second label (review I5).
func (in Incoming) Wrap() string {
	var b strings.Builder
	who := "colleague's machine"
	if in.Own {
		who = "your machine"
	}
	author := OneLine(in.Author)
	if u := OneLine(in.User); u != "" {
		author += " (" + u + ")"
	}
	fmt.Fprintf(&b, "[Room %s · from %s %s · %s · thread %s]\n%s", OneLine(in.RoomName), who, author, in.Kind, in.Thread, Quote(in.Text))
	keys := make([]string, 0, len(in.Fields))
	for k := range in.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if v := strings.TrimSpace(in.Fields[k]); v != "" {
			fmt.Fprintf(&b, "\n> %s: %s", OneLine(k), OneLine(v))
		}
	}
	for _, a := range in.Attach {
		fmt.Fprintf(&b, "\n> attachment: %s (%s) id %s", OneLine(a.Name), humanSize(a.Size), a.SHA)
	}
	return b.String()
}

// Quote prefixes every line with "> " after Clean (security review: room
// text is pasted into a terminal).
func Quote(s string) string {
	return "> " + strings.ReplaceAll(Clean(s), "\n", "\n> ")
}

// Clean keeps newlines and drops every other control character — ESC, CR,
// BEL, DEL, C1 — so text pasted into a terminal can't end bracketed paste,
// press Enter or drive the terminal.
func Clean(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' {
			return r
		}
		if unicode.IsControl(r) || r == 0x7f || r == '\u2028' || r == '\u2029' {
			return -1
		}
		return r
	}, strings.ReplaceAll(s, "\r\n", "\n"))
}

// OneLine folds newlines into spaces and drops other control characters.
func OneLine(s string) string {
	return strings.ReplaceAll(Clean(s), "\n", " ")
}

// incomingLocked builds the Incoming view of an accepted msg entry.
func (s *Service) incomingLocked(r *room, st roomlog.State, e roomlog.Entry) (Incoming, bool) {
	var m roomlog.Msg
	if e.Kind != roomlog.KindMsg || json.Unmarshal(e.Body, &m) != nil {
		return Incoming{}, false
	}
	u, _ := st.UserOf(e.Device)
	in := Incoming{Room: r.id, RoomName: st.Name, Ref: e.Ref(), Thread: e.Ref(), Device: e.Device, Author: st.Names[e.Device],
		Own: string(u) == string(s.me.User), Kind: m.Kind, Text: m.Text, Fields: m.Fields, Time: e.Time,
		User: st.UserNames[string(u)], Attach: roomlog.ValidAttachments(m.Attach)}
	for _, d := range m.Mentions {
		in.MentionsThis = in.MentionsThis || d == s.me.PeerID()
	}
	if m.Parent != "" {
		in.Thread = m.Parent
	}
	if in.Author == "" {
		in.Author = shortDev(e.Device)
	}
	return in, true
}

// recent: history arriving with a join is not news (15 minutes, writer clock).
func recent(ts string, now time.Time) bool {
	t, err := time.Parse(time.RFC3339, ts)
	return err == nil && now.Sub(t) < 15*time.Minute
}

func shortDev(d string) string {
	if len(d) > 8 {
		return d[len(d)-8:]
	}
	return d
}

// announceLocked tells OnMessage about messages from other devices that this
// sync made accepted for the first time.
func (s *Service) announceLocked(r *room, es []roomlog.Entry) {
	if s.OnMessage == nil && s.OnNotify == nil {
		return
	}
	me := s.me.PeerID()
	st := s.stateLocked(r)
	if r.told == nil {
		r.told = map[string]bool{}
	}
	fresh := map[string]bool{}
	for _, e := range es {
		if e.Kind == roomlog.KindMsg && e.Device != me && !r.told[e.Ref()] && recent(e.Time, s.now()) {
			fresh[e.Ref()] = true
		}
	}
	if len(fresh) == 0 {
		return
	}
	var out []Incoming
	for _, e := range st.Accepted {
		if fresh[e.Ref()] {
			r.told[e.Ref()] = true
			if in, ok := s.incomingLocked(r, st, e); ok {
				out = append(out, in)
			}
		}
	}
	if len(out) == 0 {
		return
	}
	cb, notify := s.OnMessage, s.OnNotify
	go func() {
		for _, in := range out {
			if notify != nil && in.MentionsThis && (in.Kind == "handoff" || in.Kind == "question") {
				who := in.Author
				if in.User != "" {
					who = in.User + " · " + in.Author
				}
				notify("Room "+OneLine(in.RoomName)+" · "+in.Kind+" for you", OneLine(who)+": "+clip(OneLine(in.Text), 120))
			}
			if cb != nil {
				cb(in)
			}
		}
	}()
}

// humanSize: 512 B, 3.4 KB, 1.2 MB.
func humanSize(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1<<20:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
}

// Message returns one accepted message of a room.
func (s *Service) Message(roomID, ref string) (Incoming, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, err := s.roomLocked(roomID)
	if err != nil {
		return Incoming{}, err
	}
	st := s.stateLocked(r)
	for _, e := range st.Accepted {
		if e.Kind == roomlog.KindMsg && e.Ref() == ref {
			if in, ok := s.incomingLocked(r, st, e); ok {
				return in, nil
			}
		}
	}
	return Incoming{}, errors.New("no such message in this room")
}

// writeLocked appends one entry of mine to an open room.
func (s *Service) writeLocked(r *room, kind string, body any) error {
	if s.closedLocked(r) {
		return errClosed
	}
	_, err := r.w.Write(kind, body)
	return err
}

func (s *Service) write(roomID, kind string, body any) error {
	s.mu.Lock()
	r, err := s.roomLocked(roomID)
	if err == nil {
		err = s.writeLocked(r, kind, body)
	}
	s.mu.Unlock()
	if err != nil {
		return err
	}
	go s.SyncAll()
	s.changed()
	return nil
}

// PostStatus writes one automatic status event (§8.1). Closed rooms get none.
func (s *Service) PostStatus(roomID string, st roomlog.Status) error {
	if st.Session == "" || st.Event == "" {
		return errors.New("status needs a session and an event")
	}
	st.Summary = clipRunes(st.Summary, 600)
	st.Title = clipRunes(st.Title, 200)
	st.Subject = clipRunes(st.Subject, 200)
	return s.write(roomID, roomlog.KindStatus, st)
}

func clipRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// msgThreadLocked: the thread a message belongs to (its parent, or itself).
func (s *Service) msgThreadLocked(r *room, ref string) (string, bool) {
	for _, e := range s.stateLocked(r).Accepted {
		if e.Kind == roomlog.KindMsg && e.Ref() == ref {
			var m roomlog.Msg
			if json.Unmarshal(e.Body, &m) == nil && m.Parent != "" {
				return m.Parent, true
			}
			return ref, true
		}
	}
	return "", false
}

// Claim takes a thread for this machine (§8.3).
func (s *Service) Claim(roomID, thread, session string) error {
	s.mu.Lock()
	r, err := s.roomLocked(roomID)
	if err == nil {
		if t, ok := s.msgThreadLocked(r, thread); !ok {
			err = errors.New("no such thread")
		} else {
			err = s.writeLocked(r, roomlog.KindClaim, roomlog.Claim{Thread: t, Session: session})
		}
	}
	s.mu.Unlock()
	if err != nil {
		return err
	}
	go s.SyncAll()
	s.changed()
	return nil
}

// Release gives a thread back (a reply ref names its thread).
func (s *Service) Release(roomID, thread string) error {
	s.mu.Lock()
	r, err := s.roomLocked(roomID)
	if err == nil {
		if t, ok := s.msgThreadLocked(r, thread); !ok {
			err = errors.New("no such thread")
		} else {
			err = s.writeLocked(r, roomlog.KindClaimRelease, roomlog.Claim{Thread: t})
		}
	}
	s.mu.Unlock()
	if err != nil {
		return err
	}
	go s.SyncAll()
	s.changed()
	return nil
}

// PickUp records that a session here took a message: a claim on its thread
// (handoffs and questions) and a picked-up receipt (§8.4, §10.5).
func (s *Service) PickUp(roomID, ref, session string) error {
	s.mu.Lock()
	r, err := s.roomLocked(roomID)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	in, ok := Incoming{}, false
	st := s.stateLocked(r)
	for _, e := range st.Accepted {
		if e.Kind == roomlog.KindMsg && e.Ref() == ref {
			in, ok = s.incomingLocked(r, st, e)
		}
	}
	if !ok {
		s.mu.Unlock()
		return errors.New("no such message in this room")
	}
	if in.Kind == "handoff" || in.Kind == "question" {
		mine := false
		for _, c := range st.Claims[in.Thread] {
			mine = mine || c.Device == s.me.PeerID()
		}
		if !mine {
			if err := s.writeLocked(r, roomlog.KindClaim, roomlog.Claim{Thread: in.Thread, Session: session}); err != nil {
				s.mu.Unlock()
				return err
			}
		}
	}
	dev, seq, _ := splitRef(ref)
	err = s.writeLocked(r, roomlog.KindReceipt, roomlog.Receipt{Lane: dev, Seq: seq, State: "picked-up"})
	r.st.Seen[ref] = true
	s.saveRoomLocked(r)
	s.mu.Unlock()
	go s.SyncAll()
	s.changed()
	return err
}

func splitRef(ref string) (string, int64, bool) {
	dev, seqStr, ok := strings.Cut(ref, ":")
	var seq int64
	if !ok || json.Unmarshal([]byte(seqStr), &seq) != nil {
		return "", 0, false
	}
	return dev, seq, true
}

// Dismiss hides an inbox item on this machine.
func (s *Service) Dismiss(roomID, ref string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, err := s.roomLocked(roomID)
	if err != nil {
		return err
	}
	r.st.Dismissed[ref] = true
	s.saveRoomLocked(r)
	s.changed()
	return nil
}

// Settings returns this machine's settings for a room.
func (s *Service) Settings(roomID string) (SettingsView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, err := s.roomLocked(roomID)
	if err != nil {
		return SettingsView{}, err
	}
	return settingsView(r.st.Settings), nil
}

func settingsView(rs RoomSettings) SettingsView {
	return SettingsView{Summaries: !rs.NoSummaries, Budget: rs.budget(), Rate: rs.rate()}
}

// SetSettings changes them (budget 1–50, rate 1–300).
func (s *Service) SetSettings(roomID string, v SettingsView) error {
	if v.Budget < 1 || v.Budget > 50 || v.Rate < 1 || v.Rate > 300 {
		return errors.New("budget is 1–50 deliveries, rate 1–300 posts an hour")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	r, err := s.roomLocked(roomID)
	if err != nil {
		return err
	}
	r.st.Settings = RoomSettings{NoSummaries: !v.Summaries, Budget: v.Budget, Rate: v.Rate}
	s.saveRoomLocked(r)
	s.changed()
	return nil
}

// UseDelivery spends one automatic delivery on a thread; false when the
// budget is used up (§10.4).
func (s *Service) UseDelivery(roomID, thread string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, err := s.roomLocked(roomID)
	if err != nil {
		return false, err
	}
	if r.st.Used == nil {
		r.st.Used = map[string]int{}
	}
	if r.st.Used[thread] >= r.st.Settings.budget()+r.st.Granted[thread] {
		return false, nil
	}
	r.st.Used[thread]++
	s.saveRoomLocked(r)
	return true, nil
}

// BudgetLeft: automatic deliveries left on a thread.
func (s *Service) BudgetLeft(roomID, thread string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.rooms[roomID]
	if r == nil {
		return 0
	}
	return r.st.Settings.budget() + r.st.Granted[thread] - r.st.Used[thread]
}

// GrantDeliveries lets a thread auto-deliver n more times.
func (s *Service) GrantDeliveries(roomID, thread string, n int) error {
	if n < 1 || n > 50 {
		return errors.New("grant 1–50")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	r, err := s.roomLocked(roomID)
	if err != nil {
		return err
	}
	if r.st.Granted == nil {
		r.st.Granted = map[string]int{}
	}
	r.st.Granted[thread] += n
	s.saveRoomLocked(r)
	s.changed()
	return nil
}

// AgentPost posts for an agent under the rate limit (§10.4): over the limit
// the post is held as a draft the user can release. Returns held=true then,
// else the new message's ref.
func (s *Service) AgentPost(roomID, kind, text, parent string, fields map[string]string, session string) (ref string, held bool, err error) {
	return s.AgentPostMsg(roomID, roomlog.Msg{Kind: kind, Text: text, Parent: parent, Fields: fields}, session)
}

// AgentPostMsg is AgentPost with attachments and mentions.
func (s *Service) AgentPostMsg(roomID string, m roomlog.Msg, session string) (ref string, held bool, err error) {
	if err := s.checkMsg(roomID, &m); err != nil {
		return "", false, err
	}
	kind, text, parent, fields := m.Kind, m.Text, m.Parent, m.Fields
	s.mu.Lock()
	r, err := s.roomLocked(roomID)
	if err != nil {
		s.mu.Unlock()
		return "", false, err
	}
	if s.closedLocked(r) {
		s.mu.Unlock()
		return "", false, errClosed
	}
	if parent != "" {
		t, ok := s.msgThreadLocked(r, parent)
		if !ok {
			s.mu.Unlock()
			return "", false, errors.New("no such thread in this room")
		}
		parent = t
	}
	if err := mentionsLocked(s.stateLocked(r), &m); err != nil {
		s.mu.Unlock()
		return "", false, err
	}
	size := len(text)
	for k, v := range fields {
		size += len(k) + len(v)
	}
	if size > roomlog.MaxBody {
		s.mu.Unlock()
		return "", false, errors.New("messages (with their fields) are limited to 16 KB")
	}
	now := s.now().UnixMilli()
	keep := r.st.Agent[:0]
	for _, t := range r.st.Agent {
		if now-t < time.Hour.Milliseconds() {
			keep = append(keep, t)
		}
	}
	r.st.Agent = keep
	if len(keep) >= r.st.Settings.rate() {
		if len(r.st.Drafts) >= maxDrafts {
			s.mu.Unlock()
			return "", false, errors.New("too many held posts in this room; ask your user to release or drop them")
		}
		r.st.Drafts = append(r.st.Drafts, Draft{ID: fmt.Sprintf("d%d", time.Now().UnixNano()), Kind: kind, Text: text, Parent: parent, Fields: fields, Session: session, At: now,
			Attach: m.Attach, Mentions: m.Mentions})
		s.saveRoomLocked(r)
		s.mu.Unlock()
		s.changed()
		return "", true, nil
	}
	e, err := r.w.Write(roomlog.KindMsg, roomlog.Msg{Kind: kind, Text: text, Parent: parent, Fields: fields, Attach: m.Attach, Mentions: m.Mentions})
	if err == nil {
		r.st.Agent = append(r.st.Agent, now)
		s.saveRoomLocked(r)
	}
	s.mu.Unlock()
	if err != nil {
		return "", false, err
	}
	go s.SyncAll()
	s.changed()
	return e.Ref(), false, nil
}

// ReleaseDraft posts a held draft (the user's decision, not rate-limited);
// DropDraft deletes it.
func (s *Service) ReleaseDraft(roomID, id string) error { return s.takeDraft(roomID, id, true) }
func (s *Service) DropDraft(roomID, id string) error    { return s.takeDraft(roomID, id, false) }

func (s *Service) takeDraft(roomID, id string, post bool) error {
	s.mu.Lock()
	r, err := s.roomLocked(roomID)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	var d *Draft
	for i := range r.st.Drafts {
		if r.st.Drafts[i].ID == id {
			dd := r.st.Drafts[i]
			d = &dd
			r.st.Drafts = append(r.st.Drafts[:i], r.st.Drafts[i+1:]...)
			break
		}
	}
	if d == nil {
		s.mu.Unlock()
		return errors.New("no such draft")
	}
	if post {
		for _, a := range d.Attach {
			if _, ok := s.AttachmentPath(roomID, a.SHA); !ok {
				r.st.Drafts = append(r.st.Drafts, *d) // stays held until the user drops it
				s.mu.Unlock()
				return fmt.Errorf("attachment %s is no longer on this machine (attachments are kept 7 days); drop this post", roomlog.AttachName(a.Name))
			}
		}
		err = s.writeLocked(r, roomlog.KindMsg, roomlog.Msg{Kind: d.Kind, Text: d.Text, Parent: d.Parent, Fields: d.Fields, Attach: d.Attach, Mentions: d.Mentions})
	}
	if err == nil {
		s.saveRoomLocked(r)
	}
	s.mu.Unlock()
	if post && err == nil {
		go s.SyncAll()
	}
	s.changed()
	return err
}

// LiveView is one running session in the room (the live strip, §11).
type LiveView struct {
	roomlog.Status
	Device string `json:"device"`
	Mine   bool   `json:"mine"`  // this machine
	Owned  bool   `json:"owned"` // one of my machines
	Online bool   `json:"online"`
}
