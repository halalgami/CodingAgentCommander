package roomlog

import (
	"strings"
	"testing"
)

func TestAttachNameSafe(t *testing.T) {
	cases := map[string]string{
		"../../etc/passwd":       "passwd",
		`C:\Users\x\notes.txt`:   "notes.txt",
		"a\x1b[201~b.png":        "a[201~b.png",
		"":                       "file",
		"..":                     "file",
		".":                      "file",
		strings.Repeat("é", 300): strings.Repeat("é", 100),
	}
	for in, want := range cases {
		if got := AttachName(in); got != want {
			t.Errorf("AttachName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidAttachments(t *testing.T) {
	sha := strings.Repeat("ab", 32)
	ok := []Attachment{{Name: "a.txt", Size: 10, SHA: sha}}
	if got := ValidAttachments(ok); len(got) != 1 {
		t.Fatalf("valid dropped: %v", got)
	}
	bad := []Attachment{
		{Name: "big", Size: MaxAttachment + 1, SHA: sha},
		{Name: "neg", Size: -1, SHA: sha},
		{Name: "sha", Size: 1, SHA: "zz"},
		{Name: "../x", Size: 1, SHA: strings.Repeat("cd", 32)},
	}
	got := ValidAttachments(bad)
	if len(got) != 1 || got[0].Name != "x" {
		t.Fatalf("invalid kept or name not cleaned: %+v", got)
	}
	var many []Attachment
	for i := 0; i < MaxAttachCount+3; i++ {
		many = append(many, Attachment{Name: "f", Size: 1, SHA: sha})
	}
	if got := ValidAttachments(many); len(got) != MaxAttachCount {
		t.Fatalf("count cap: %d", len(got))
	}
	var heavy []Attachment
	for i := 0; i < 6; i++ {
		heavy = append(heavy, Attachment{Name: "f", Size: MaxAttachment, SHA: sha})
	}
	total := int64(0)
	for _, a := range ValidAttachments(heavy) {
		total += a.Size
	}
	if total > MaxAttachTotal {
		t.Fatalf("total cap broken: %d", total)
	}
}

func TestProfileNames(t *testing.T) {
	a, b := closeRoom(t)
	b.post(t, KindProfile, Profile{Name: "Hasan"})
	syncTo(t, b, a)
	st := Replay(a.lane.All())
	if got := st.UserNames[string(b.id.User)]; got != "Hasan" {
		t.Fatalf("user name = %q (%v)", got, st.UserNames)
	}
	b.post(t, KindProfile, Profile{Name: strings.Repeat("x", 200) + "\x1b"})
	syncTo(t, b, a)
	st = Replay(a.lane.All())
	if n := st.UserNames[string(b.id.User)]; len([]rune(n)) > MaxUserName || strings.ContainsRune(n, 0x1b) || n == "" {
		t.Fatalf("name not bounded/cleaned: %q", n)
	}
}
