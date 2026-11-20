package rooms

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/halalgami/CodingAgentCommander/internal/rooms/netsel"
	"github.com/halalgami/CodingAgentCommander/internal/rooms/roomlog"
)

func tempFile(t *testing.T, name string, b []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func msgByText(t *testing.T, m *machine, id, text string) MsgView {
	t.Helper()
	for _, th := range viewOf(t, m, id).Threads {
		for _, mv := range append([]MsgView{th.Root}, th.Replies...) {
			if mv.Text == text {
				return mv
			}
		}
	}
	return MsgView{}
}

func TestAttachmentTravelsByHash(t *testing.T) {
	a, _, c, id := threeMachines(t)
	body := []byte("hello attachment\n")
	att, err := a.StoreAttachment(id, tempFile(t, "notes.txt", body))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.PostMsg(id, roomlog.Msg{Kind: "note", Text: "see file", Attach: []roomlog.Attachment{att}}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "msg on c", func() bool { return refOf(t, c, id, "see file") != "" })
	mv := msgByText(t, c, id, "see file")
	if len(mv.Attach) != 1 || mv.Attach[0].Name != "notes.txt" || mv.Attach[0].Have {
		t.Fatalf("attach view = %+v", mv.Attach)
	}
	p, err := c.FetchAttachment(context.Background(), id, att.SHA)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	if !bytes.Equal(got, body) {
		t.Fatalf("bytes = %q", got)
	}
	if mv := msgByText(t, c, id, "see file"); !mv.Attach[0].Have {
		t.Fatal("Have not set after fetch")
	}
}

func TestBlobRefusesUnreferencedSHA(t *testing.T) {
	a, _, c, id := threeMachines(t)
	att, err := a.StoreAttachment(id, tempFile(t, "secret.txt", []byte("never posted")))
	if err != nil {
		t.Fatal(err)
	}
	a.Post(id, "note", "sync me", "", nil)
	waitFor(t, "msg on c", func() bool { return refOf(t, c, id, "sync me") != "" })
	if _, err := c.FetchAttachment(context.Background(), id, att.SHA); err == nil {
		t.Fatal("fetched an attachment no message names")
	}
}

func TestFetchRejectsWrongBytes(t *testing.T) {
	a, _, c, id := threeMachines(t)
	att, _ := a.StoreAttachment(id, tempFile(t, "x.bin", []byte("good bytes")))
	a.PostMsg(id, roomlog.Msg{Kind: "note", Text: "bin", Attach: []roomlog.Attachment{att}})
	waitFor(t, "msg on c", func() bool { return refOf(t, c, id, "bin") != "" })
	os.WriteFile(filepath.Join(a.dir, id, "attachments", att.SHA), []byte("evil bytes"), 0o600)
	if _, err := c.FetchAttachment(context.Background(), id, att.SHA); err == nil {
		t.Fatal("accepted bytes that don't match the hash")
	}
	if _, err := os.Stat(filepath.Join(c.dir, id, "attachments", att.SHA)); err == nil {
		t.Fatal("bad bytes stored")
	}
}

func TestAttachmentCaps(t *testing.T) {
	a, _, _, id := threeMachines(t)
	if _, err := a.StoreAttachment(id, tempFile(t, "big", make([]byte, roomlog.MaxAttachment+1))); err == nil {
		t.Fatal("over 2 MB stored")
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "real"), []byte("x"), 0o600)
	os.Symlink(filepath.Join(dir, "real"), filepath.Join(dir, "link"))
	if _, err := a.StoreAttachment(id, filepath.Join(dir, "link")); err == nil {
		t.Fatal("followed a symlink")
	}
	var many []roomlog.Attachment
	for i := 0; i < roomlog.MaxAttachCount+1; i++ {
		att, err := a.StoreAttachment(id, tempFile(t, "f", []byte{byte(i)}))
		if err != nil {
			t.Fatal(err)
		}
		many = append(many, att)
	}
	if _, err := a.PostMsg(id, roomlog.Msg{Kind: "note", Text: "many", Attach: many}); err == nil {
		t.Fatal("9 attachments posted")
	}
	ghost := roomlog.Attachment{Name: "g", Size: 1, SHA: strings.Repeat("ab", 32)}
	if _, err := a.PostMsg(id, roomlog.Msg{Kind: "note", Text: "ghost", Attach: []roomlog.Attachment{ghost}}); err == nil {
		t.Fatal("posted an attachment this machine doesn't hold")
	}
	att, _ := a.StoreAttachment(id, tempFile(t, "only.txt", []byte("x")))
	if _, err := a.PostMsg(id, roomlog.Msg{Kind: "note", Attach: []roomlog.Attachment{att}}); err != nil {
		t.Fatalf("attachment without text refused: %v", err)
	}
}

func TestEvictAttachments(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, strings.Repeat("aa", 32))
	keep := filepath.Join(dir, strings.Repeat("bb", 32))
	other := filepath.Join(dir, "notes.txt")
	for _, p := range []string{old, keep, other} {
		os.WriteFile(p, []byte("1234"), 0o600)
	}
	past := time.Now().Add(-8 * 24 * time.Hour)
	os.Chtimes(old, past, past)
	os.Chtimes(other, past, past)
	evictAttachments(dir, time.Now(), 1<<20)
	if _, err := os.Stat(old); err == nil {
		t.Fatal("8-day-old attachment kept")
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatal("fresh attachment evicted")
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatal("touched a file that isn't an attachment")
	}
	newer := filepath.Join(dir, strings.Repeat("cc", 32))
	os.WriteFile(newer, []byte("5678"), 0o600)
	os.Chtimes(keep, time.Now().Add(-time.Hour), time.Now().Add(-time.Hour))
	evictAttachments(dir, time.Now(), 6) // over the cap: oldest goes first
	if _, err := os.Stat(keep); err == nil {
		t.Fatal("oldest kept over the cap")
	}
	if _, err := os.Stat(newer); err != nil {
		t.Fatal("newest evicted")
	}
}

func TestMentionsHighlightAndNotify(t *testing.T) {
	a, b, c, id := threeMachines(t)
	var mu sync.Mutex
	var got []string
	a.OnNotify = func(title, body string) { mu.Lock(); got = append(got, title+"|"+body); mu.Unlock() }
	var bGot int
	b.OnNotify = func(string, string) { mu.Lock(); bGot++; mu.Unlock() }
	if _, err := c.PostMsg(id, roomlog.Msg{Kind: "question", Text: "can you look?", Mentions: []string{a.me.PeerID()}}); err != nil {
		t.Fatal(err)
	}
	c.PostMsg(id, roomlog.Msg{Kind: "note", Text: "fyi only", Mentions: []string{a.me.PeerID()}})
	waitFor(t, "a notified", func() bool { mu.Lock(); defer mu.Unlock(); return len(got) == 1 })
	waitFor(t, "b has it", func() bool { return refOf(t, b, id, "can you look?") != "" })
	if mv := msgByText(t, a, id, "can you look?"); !mv.MentionsMe || len(mv.Mentions) != 1 {
		t.Fatalf("mention view = %+v", mv)
	}
	if mv := msgByText(t, b, id, "can you look?"); !mv.MentionsMe {
		t.Fatal("my other machine doesn't highlight a mention of my machine")
	}
	time.Sleep(200 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if bGot != 0 {
		t.Fatal("unmentioned machine notified")
	}
	if !strings.Contains(got[0], "can you look?") {
		t.Fatalf("notification = %q", got[0])
	}
	if _, err := c.PostMsg(id, roomlog.Msg{Kind: "note", Text: "x", Mentions: []string{"12D3KooWnotamember"}}); err == nil {
		t.Fatal("mention of a non-member accepted")
	}
}

func TestUserNameShared(t *testing.T) {
	a, _, c, id := threeMachines(t)
	a.Post(id, "note", "from hussain", "", nil)
	waitFor(t, "user name on c", func() bool { return msgByText(t, c, id, "from hussain").User == "Hussain" })
}

func TestTailnetCandidates(t *testing.T) {
	known := []string{"/ip4/100.64.0.5/tcp/4100", "/ip4/100.64.0.5/udp/4101/quic-v1", "/ip4/192.168.1.9/tcp/4100"}
	peers := []netsel.TailPeer{
		{Name: "mbp", IPs: []string{"100.70.1.2", "fd7a:115c:a1e0::1"}, Online: true},
		{Name: "off", IPs: []string{"100.70.9.9"}, Online: false},
	}
	got := tailnetCandidates(known, peers)
	want := []string{
		"/ip4/100.70.1.2/tcp/4100", "/ip4/100.70.1.2/udp/4101/quic-v1",
		"/ip6/fd7a:115c:a1e0::1/tcp/4100", "/ip6/fd7a:115c:a1e0::1/udp/4101/quic-v1",
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v", got)
	}
	if len(tailnetCandidates(nil, peers)) != 0 {
		t.Fatal("no known port: nothing to dial")
	}
}

func TestThreadOutOfBudgetAndGrant(t *testing.T) {
	a, _, c, id := threeMachines(t)
	ref, _ := a.PostRef(id, "question", "loop?", "", nil)
	_ = c
	for i := 0; i < 5; i++ {
		if ok, err := a.UseDelivery(id, ref); !ok || err != nil {
			t.Fatalf("delivery %d: %v %v", i, ok, err)
		}
	}
	th := func() Thread {
		for _, x := range viewOf(t, a, id).Threads {
			if x.Root.Ref == ref {
				return x
			}
		}
		return Thread{}
	}
	if !th().OutOfBudget {
		t.Fatal("not flagged after 5 deliveries")
	}
	a.GrantDeliveries(id, ref, 5)
	if th().OutOfBudget {
		t.Fatal("still flagged after grant")
	}
}

// Review I4: a held draft whose attachment is gone is not posted.
func TestReleaseDraftNeedsItsAttachments(t *testing.T) {
	a, _, _, id := threeMachines(t)
	a.SetSettings(id, SettingsView{Summaries: true, Budget: 5, Rate: 1})
	a.AgentPost(id, "note", "first", "", nil, "s")
	att, _ := a.StoreAttachment(id, tempFile(t, "x.txt", []byte("x")))
	_, held, err := a.AgentPostMsg(id, roomlog.Msg{Kind: "note", Text: "held", Attach: []roomlog.Attachment{att}}, "s")
	if !held || err != nil {
		t.Fatalf("held=%v err=%v", held, err)
	}
	os.Remove(filepath.Join(a.dir, id, "attachments", att.SHA))
	d := viewOf(t, a, id).Drafts[0]
	if err := a.ReleaseDraft(id, d.ID); err == nil {
		t.Fatal("released a draft whose attachment is gone")
	}
}

// Review I3: one peer that never answers must not use up the whole fetch.
func TestFetchTimeoutIsPerPeer(t *testing.T) {
	if perPeerFetch >= blobFetchBudget/2 {
		t.Fatalf("per-peer %v vs budget %v", perPeerFetch, blobFetchBudget)
	}
}

// Review I2: the hash index is built once per state, not per request.
func TestNamedIndexCached(t *testing.T) {
	a, _, _, id := threeMachines(t)
	att, _ := a.StoreAttachment(id, tempFile(t, "x.txt", []byte("x")))
	a.PostMsg(id, roomlog.Msg{Kind: "note", Text: "n", Attach: []roomlog.Attachment{att}})
	a.mu.Lock()
	r := a.rooms[id]
	_, ok := a.namedLocked(r, att.SHA)
	ix := r.named
	a.mu.Unlock()
	if !ok || ix == nil {
		t.Fatal("no cached index")
	}
}

// Review minor: dial suffixes only from tailnet addresses.
func TestTailnetCandidatesOnlyFromTailnetAddrs(t *testing.T) {
	got := tailnetCandidates([]string{"/ip4/192.168.1.9/tcp/22", "/ip4/100.64.0.5/tcp/4100"},
		[]netsel.TailPeer{{IPs: []string{"100.70.1.2"}, Online: true}})
	if len(got) != 1 || got[0] != "/ip4/100.70.1.2/tcp/4100" {
		t.Fatalf("got %v", got)
	}
}
