package hookmgr

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func read(t *testing.T, p string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return m
}

func TestInstallPreservesOtherContentAndRemoveIsSurgical(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	// Pre-existing content incl. an unrelated user Stop hook.
	os.WriteFile(p, []byte(`{
      "permissions": {"allow": ["Bash(ls)"]},
      "hooks": {"Stop": [{"hooks":[{"type":"command","command":"echo user-hook"}]}]}
    }`), 0o600)

	if err := Install(p, 9876, "tok"); err != nil {
		t.Fatalf("Install: %v", err)
	}
	m := read(t, p)
	// unrelated content preserved
	if _, ok := m["permissions"]; !ok {
		t.Error("permissions dropped")
	}
	// both the user hook and ours are present
	raw, _ := json.Marshal(m)
	s := string(raw)
	if !strings.Contains(s, "echo user-hook") {
		t.Error("user hook dropped")
	}
	if !strings.Contains(s, Sentinel) || !strings.Contains(s, "9876") {
		t.Error("commander hook not installed with sentinel+port")
	}
	// The notify URL must carry the auth token so a stray process can't spoof it.
	if !strings.Contains(s, "token=tok") {
		t.Error("notify command missing auth token")
	}

	// Idempotent: install again → still exactly one commander block.
	if err := Install(p, 9876, "tok"); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(mustMarshal(t, read(t, p))), Sentinel); n != 1 {
		t.Errorf("expected 1 commander hook after re-install, got %d", n)
	}

	// Remove: our block gone, user hook stays.
	if err := Remove(p); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	s2 := string(mustMarshal(t, read(t, p)))
	if strings.Contains(s2, Sentinel) {
		t.Error("commander hook not removed")
	}
	if !strings.Contains(s2, "echo user-hook") {
		t.Error("user hook lost during Remove")
	}
}

func TestInstallCreatesFileIfAbsent(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "settings.json")
	if err := Install(p, 4321, "tok"); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if !strings.Contains(string(mustMarshal(t, read(t, p))), Sentinel) {
		t.Error("hook not installed into new file")
	}
}

func mustMarshal(t *testing.T, m map[string]any) []byte {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
