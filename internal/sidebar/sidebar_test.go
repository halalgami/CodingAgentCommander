package sidebar

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func ids(l Layout, key string) []string { return l.Order[key] }

func eq(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) == 0 && len(want) == 0 {
		return
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestMergePlacesNewWindowsInUngrouped(t *testing.T) {
	s := Open("")
	l, err := s.Merge([]Window{{"@1", "/a"}, {"@2", "/b"}})
	if err != nil {
		t.Fatal(err)
	}
	eq(t, ids(l, Ungrouped), []string{"@1", "@2"})
}

func TestMergeKeepsExistingOrder(t *testing.T) {
	s := Open("")
	_ = s.Set(Layout{Order: map[string][]string{Ungrouped: {"@2", "@1"}}})
	l, _ := s.Merge([]Window{{"@1", "/a"}, {"@2", "/b"}, {"@3", "/c"}})
	eq(t, ids(l, Ungrouped), []string{"@2", "@1", "@3"})
}

func TestMergePrunesDeadAndReusedIDs(t *testing.T) {
	s := Open("")
	_ = s.Set(Layout{
		Groups: []Group{{ID: "g_1", Name: "A"}},
		Order:  map[string][]string{"g_1": {"@1"}},
	})
	l, _ := s.Merge([]Window{{"@2", "/b"}})
	eq(t, ids(l, "g_1"), nil)
	eq(t, ids(l, Ungrouped), []string{"@2"})
	l, _ = s.Merge([]Window{{"@1", "/elsewhere"}, {"@2", "/b"}})
	eq(t, ids(l, "g_1"), nil)
	eq(t, ids(l, Ungrouped), []string{"@2", "@1"})
}

func TestMergeRememberedFolderJoinsGroup(t *testing.T) {
	s := Open("")
	_ = s.Set(Layout{Groups: []Group{{ID: "g_1", Name: "Ayina", Remember: true, Folders: []string{"/p/ayina"}}}})
	l, _ := s.Merge([]Window{{"@1", "/p/ayina/apps"}})
	eq(t, ids(l, "g_1"), []string{"@1"})
}

func TestMergeLongestRememberedFolderWins(t *testing.T) {
	s := Open("")
	_ = s.Set(Layout{Groups: []Group{
		{ID: "g_1", Name: "P", Remember: true, Folders: []string{"/p"}},
		{ID: "g_2", Name: "Ayina", Remember: true, Folders: []string{"/p/ayina"}},
	}})
	l, _ := s.Merge([]Window{{"@1", "/p/ayina/x"}})
	eq(t, ids(l, "g_2"), []string{"@1"})
	eq(t, ids(l, "g_1"), nil)
}

func TestMergeIgnoresFoldersWhenRememberOff(t *testing.T) {
	s := Open("")
	_ = s.Set(Layout{Groups: []Group{
		{ID: "g_1", Name: "P", Remember: true, Folders: []string{"/p"}},
		{ID: "g_2", Name: "Ayina", Remember: false, Folders: []string{"/p/ayina"}},
	}})
	l, _ := s.Merge([]Window{{"@1", "/p/ayina/x"}})
	eq(t, ids(l, "g_1"), []string{"@1"})
}

func TestFolderMatchIsPathAware(t *testing.T) {
	s := Open("")
	_ = s.Set(Layout{Groups: []Group{{ID: "g_1", Name: "Ayina", Remember: true, Folders: []string{"/p/ayina"}}}})
	l, _ := s.Merge([]Window{{"@1", "/p/ayina2"}})
	eq(t, ids(l, Ungrouped), []string{"@1"})
}

func TestNormalizeDropsDuplicatesAndUnknownSections(t *testing.T) {
	l := Normalize(Layout{
		Groups: []Group{{ID: "g_1", Name: "A"}, {ID: "g_2", Name: "B"}},
		Order: map[string][]string{
			Ungrouped: {"@1"},
			"g_1":     {"@1", "@2"},
			"g_zz":    {"@3"},
		},
	})
	eq(t, ids(l, Ungrouped), []string{"@1", "@3"})
	eq(t, ids(l, "g_1"), []string{"@2"})
	if _, ok := l.Order["g_2"]; !ok {
		t.Fatal("every group needs an order entry")
	}
	if _, ok := l.Order["g_zz"]; ok {
		t.Fatal("unknown section kept")
	}
}

func TestOpenCorruptBacksUp(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sidebar.json")
	_ = os.WriteFile(p, []byte("{nope"), 0o600)
	s := Open(p)
	if l := s.Get(); len(l.Groups) != 0 || len(l.Order[Ungrouped]) != 0 {
		t.Fatalf("want empty layout, got %+v", l)
	}
	b, err := os.ReadFile(p + ".corrupt")
	if err != nil || string(b) != "{nope" {
		t.Fatalf("backup missing or wrong: %q %v", b, err)
	}
}

func TestSetWritesAtomically0600(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sub", "sidebar.json")
	s := Open(p)
	want := Layout{Groups: []Group{{ID: "g_1", Name: "A", Collapsed: true}}, Order: map[string][]string{"g_1": {"@1"}}}
	if err := s.Set(want); err != nil {
		t.Fatal(err)
	}
	got := Open(p).Get()
	if got.Groups[0].Name != "A" || !got.Groups[0].Collapsed {
		t.Fatalf("round trip lost data: %+v", got)
	}
	eq(t, ids(got, "g_1"), []string{"@1"})
	fi, _ := os.Stat(p)
	if runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", fi.Mode().Perm())
	}
}

func TestGetIsACopy(t *testing.T) {
	s := Open("")
	_ = s.Set(Layout{Order: map[string][]string{Ungrouped: {"@1"}}})
	l := s.Get()
	l.Order[Ungrouped][0] = "@9"
	eq(t, ids(s.Get(), Ungrouped), []string{"@1"})
}
