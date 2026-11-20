package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestRoomPostAttachAndFetch(t *testing.T) {
	m := newMCPRig(t)
	os.WriteFile(filepath.Join(m.dir, "notes.md"), []byte("# plan\nstep one\n"), 0o644)
	txt, isErr := m.tool(t, "room_post", map[string]any{"kind": "note", "text": "plan attached", "attach": []string{"notes.md"}})
	if isErr {
		t.Fatalf("post with attachment: %s", txt)
	}
	read, _ := m.tool(t, "room_read", nil)
	id := regexp.MustCompile(`attachment: notes\.md \(16 B\) id ([0-9a-f]{64})`).FindStringSubmatch(read)
	if id == nil {
		t.Fatalf("room_read lacks the attachment: %s", read)
	}
	got, isErr := m.tool(t, "room_fetch", map[string]any{"attachment": id[1]})
	if isErr {
		t.Fatalf("fetch: %s", got)
	}
	want := filepath.Join(m.dir, ".commander", "attachments", id[1][:8]+"-notes.md")
	if !strings.Contains(got, want) || !strings.Contains(got, "> step one") {
		t.Fatalf("fetch result: %s", got)
	}
	if b, err := os.ReadFile(want); err != nil || string(b) != "# plan\nstep one\n" {
		t.Fatalf("saved copy: %q %v", b, err)
	}
	ex, _ := os.ReadFile(filepath.Join(m.dir, ".git", "info", "exclude"))
	if !strings.Contains(string(ex), "\n.commander/\n") && !strings.HasPrefix(string(ex), ".commander/\n") {
		t.Fatalf("exclude not updated: %q", ex)
	}
	m.tool(t, "room_fetch", map[string]any{"attachment": id[1]})
	if ex2, _ := os.ReadFile(filepath.Join(m.dir, ".git", "info", "exclude")); strings.Count(string(ex2), ".commander/") != 1 {
		t.Fatalf("exclude line duplicated: %q", ex2)
	}
}

func TestRoomPostAttachStaysInProject(t *testing.T) {
	m := newMCPRig(t)
	outside := filepath.Join(t.TempDir(), "secret.txt")
	os.WriteFile(outside, []byte("s"), 0o600)
	for _, p := range []string{outside, "../secret.txt", "link.txt"} {
		if p == "link.txt" {
			os.Symlink(outside, filepath.Join(m.dir, "link.txt"))
		}
		if txt, isErr := m.tool(t, "room_post", map[string]any{"kind": "note", "text": "x", "attach": []string{p}}); !isErr || !strings.Contains(txt, "project") {
			t.Fatalf("attach %s: %v %s", p, isErr, txt)
		}
	}
	if txt, isErr := m.tool(t, "room_fetch", map[string]any{"attachment": strings.Repeat("ab", 32)}); !isErr {
		t.Fatalf("fetch of an unknown id: %s", txt)
	}
}

func TestRoomPostMentionsByMachineName(t *testing.T) {
	m := newMCPRig(t)
	txt, isErr := m.tool(t, "room_post", map[string]any{"kind": "question", "text": "who?", "mentions": []string{"no-such-machine"}})
	if !isErr || !strings.Contains(txt, "no-such-machine") {
		t.Fatalf("unknown mention: %v %s", isErr, txt)
	}
	txt, isErr = m.tool(t, "room_post", map[string]any{"kind": "question", "text": "me?", "mentions": []string{deviceName()}})
	if isErr {
		t.Fatalf("mention of this machine: %s", txt)
	}
}

func TestSendRoomPostWithFilesAndSave(t *testing.T) {
	old := revealDir
	var opened []string
	revealDir = func(d string) error { opened = append(opened, d); return nil }
	t.Cleanup(func() { revealDir = old })
	a := setUpRooms(t)
	dir := committedRepo(t, filepath.Join(realpath(t, t.TempDir()), "p"))
	r, err := a.ShareProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(t.TempDir(), "shot.png")
	os.WriteFile(f, []byte("png-bytes"), 0o600)
	if err := a.SendRoomPost(r.ID, RoomPost{Kind: "note", Text: "", Files: []string{f}}); err != nil {
		t.Fatal(err)
	}
	s, _ := a.rs()
	v, _ := s.View(r.ID)
	if len(v.Threads) == 0 || len(v.Threads[0].Root.Attach) != 1 {
		t.Fatalf("threads = %+v", v.Threads)
	}
	sha := v.Threads[0].Root.Attach[0].SHA
	p, err := a.SaveRoomAttachment(r.ID, sha)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(p, filepath.Join(dir, ".commander", "attachments")) {
		t.Fatalf("saved at %s", p)
	}
	if len(opened) != 1 || opened[0] != filepath.Dir(p) {
		t.Fatalf("revealed %v", opened)
	}
	if err := a.SendRoomPost(r.ID, RoomPost{Kind: "note", Text: "x", Mentions: []string{"12D3KooWnotinroom"}}); err == nil {
		t.Fatal("mention outside the room accepted")
	}
	if err := a.GrantThread(r.ID, v.Threads[0].Root.Ref); err != nil {
		t.Fatal(err)
	}
}

// Review C1: a tracked .commander/attachments symlink must not redirect
// fetched attachments outside the project.
func TestFetchRefusesSymlinkedAttachmentsDir(t *testing.T) {
	m := newMCPRig(t)
	os.WriteFile(filepath.Join(m.dir, "a.txt"), []byte("payload"), 0o644)
	m.tool(t, "room_post", map[string]any{"kind": "note", "text": "f", "attach": []string{"a.txt"}})
	read, _ := m.tool(t, "room_read", nil)
	id := regexp.MustCompile(`id ([0-9a-f]{64})`).FindStringSubmatch(read)[1]
	outside := t.TempDir()
	os.MkdirAll(filepath.Join(m.dir, ".commander"), 0o755)
	os.Symlink(outside, filepath.Join(m.dir, ".commander", "attachments"))
	if txt, isErr := m.tool(t, "room_fetch", map[string]any{"attachment": id}); !isErr {
		t.Fatalf("fetch through a symlinked folder: %s", txt)
	}
	if ents, _ := os.ReadDir(outside); len(ents) != 0 {
		t.Fatalf("wrote outside the project: %v", ents)
	}
	os.Remove(filepath.Join(m.dir, ".commander", "attachments"))
	os.Symlink(outside, filepath.Join(m.dir, ".commander2"))
	os.Remove(filepath.Join(m.dir, ".commander"))
	os.Symlink(filepath.Join(m.dir, ".commander2"), filepath.Join(m.dir, ".commander"))
	if txt, isErr := m.tool(t, "room_fetch", map[string]any{"attachment": id}); !isErr {
		t.Fatalf("fetch through a symlinked .commander: %s", txt)
	}
	if ents, _ := os.ReadDir(outside); len(ents) != 0 {
		t.Fatalf("wrote outside the project: %v", ents)
	}
}

// Review minor: a stale or crafted file at the destination is replaced, not
// handed to the agent.
func TestFetchReplacesWrongExistingCopy(t *testing.T) {
	m := newMCPRig(t)
	os.WriteFile(filepath.Join(m.dir, "a.txt"), []byte("payload"), 0o644)
	m.tool(t, "room_post", map[string]any{"kind": "note", "text": "f", "attach": []string{"a.txt"}})
	read, _ := m.tool(t, "room_read", nil)
	id := regexp.MustCompile(`id ([0-9a-f]{64})`).FindStringSubmatch(read)[1]
	dst := filepath.Join(m.dir, ".commander", "attachments", id[:8]+"-a.txt")
	os.MkdirAll(filepath.Dir(dst), 0o700)
	os.WriteFile(dst, []byte("evil!!!"), 0o600) // same size, other bytes
	m.tool(t, "room_fetch", map[string]any{"attachment": id})
	if b, _ := os.ReadFile(dst); string(b) != "payload" {
		t.Fatalf("kept the wrong copy: %q", b)
	}
}

// Review minor: a colleague's machine with my machine's name can't take my mention.
func TestMentionNameMatchingTwoMachinesRefused(t *testing.T) {
	if _, err := pickMention(map[string][]string{"mini": {"a", "b"}}, "Mini", nil); err == nil {
		t.Fatal("ambiguous name accepted")
	}
	if d, err := pickMention(map[string][]string{"mini": {"a"}}, "MINI", nil); err != nil || d != "a" {
		t.Fatalf("%v %v", d, err)
	}
}
