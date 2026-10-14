package hookmgr

import (
	"encoding/json"
	"errors"
	"fmt"
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

	if err := Install(p, 9876, "tok", false); err != nil {
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
	if err := Install(p, 9876, "tok", false); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(mustMarshal(t, read(t, p))), Sentinel); n != len(Events) {
		t.Errorf("expected %d commander hooks after re-install, got %d", len(Events), n)
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
	if err := Install(p, 4321, "tok", false); err != nil {
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

func groupsOf(t *testing.T, hooks map[string]any, ev string) []any {
	t.Helper()
	g, _ := hooks[ev].([]any)
	return g
}

func commanderCount(groups []any) int {
	n := 0
	for _, g := range groups {
		if isCommanderGroup(g) {
			n++
		}
	}
	return n
}

func TestInstallSubscribesEveryEventOnceAndKeepsUserHooks(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	os.WriteFile(p, []byte(`{"hooks":{
	  "PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"echo user-pre"}]}],
	  "Notification":[{"hooks":[{"type":"command","command":"say done"}]}]
	}}`), 0o600)
	for i := 0; i < 2; i++ { // twice: idempotent
		if err := Install(p, 9876, "tok", false); err != nil {
			t.Fatal(err)
		}
	}
	hooks := read(t, p)["hooks"].(map[string]any)
	for _, ev := range Events {
		if n := commanderCount(groupsOf(t, hooks, ev)); n != 1 {
			t.Errorf("%s: %d commander groups, want 1", ev, n)
		}
	}
	userPre, _ := json.Marshal(groupsOf(t, hooks, "PreToolUse")[0])
	if string(userPre) != `{"hooks":[{"command":"echo user-pre","type":"command"}],"matcher":"Bash"}` {
		t.Errorf("user PreToolUse group changed: %s", userPre)
	}
	userNote, _ := json.Marshal(groupsOf(t, hooks, "Notification")[0])
	if string(userNote) != `{"hooks":[{"command":"say done","type":"command"}]}` {
		t.Errorf("user Notification group changed: %s", userNote)
	}
}

func TestRemoveLeavesExactlyTheUserHooks(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	os.WriteFile(p, []byte(`{"hooks":{
	  "PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"echo user-pre"}]}],
	  "SessionStart":[]
	}}`), 0o600)
	if err := Install(p, 9876, "tok", false); err != nil {
		t.Fatal(err)
	}
	if err := Remove(p); err != nil {
		t.Fatal(err)
	}
	got := string(mustMarshal(t, read(t, p)))
	want := `{"hooks":{"PreToolUse":[{"hooks":[{"command":"echo user-pre","type":"command"}],"matcher":"Bash"}],"SessionStart":[]}}`
	if got != want {
		t.Errorf("after Remove:\n got %s\nwant %s", got, want)
	}
}

// An older or crashed build may have left a Commander group under an event
// this build no longer installs; Remove must still sweep it.
func TestRemoveSweepsCommanderGroupsUnderAnyEvent(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	os.WriteFile(p, []byte(`{"hooks":{"SubagentStop":[{"hooks":[{"type":"command","command":"curl x # `+Sentinel+`"}]}]}}`), 0o600)
	if err := Remove(p); err != nil {
		t.Fatal(err)
	}
	if got := string(mustMarshal(t, read(t, p))); got != `{}` {
		t.Errorf("after Remove: %s", got)
	}
}

func TestInstallSkipsOnlyAMalformedEvent(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	os.WriteFile(p, []byte(`{"hooks":{"Notification":{"oops":true}}}`), 0o600)
	err := Install(p, 9876, "tok", false)
	var se *SkippedError
	if !errors.As(err, &se) || len(se.Events) != 1 || se.Events[0] != "Notification" || !strings.Contains(err.Error(), "hooks.Notification is not an array") {
		t.Fatalf("err = %v", err)
	}
	hooks := read(t, p)["hooks"].(map[string]any)
	for _, ev := range Events {
		want := 1
		if ev == "Notification" {
			want = 0
		}
		if n := commanderCount(groupsOf(t, hooks, ev)); n != want {
			t.Errorf("%s: %d commander groups, want %d", ev, n, want)
		}
	}
	if got, _ := json.Marshal(hooks["Notification"]); string(got) != `{"oops":true}` {
		t.Errorf("Notification value changed: %s", got)
	}
}

func TestInstallRefusesAMalformedHooksRoot(t *testing.T) {
	for name, orig := range map[string]string{"string": `{"hooks":"nope"}`, "invalid json": `{"hooks":`} {
		p := filepath.Join(t.TempDir(), "settings.json")
		os.WriteFile(p, []byte(orig), 0o600)
		if err := Install(p, 9876, "tok", false); err == nil {
			t.Errorf("%s: no error", name)
		}
		if b, _ := os.ReadFile(p); string(b) != orig {
			t.Errorf("%s: file changed: %s", name, b)
		}
	}
}

func TestNotificationBlockPreservesUserHooks(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	os.WriteFile(p, []byte(`{"hooks":{
	  "Notification":[{"matcher":"","hooks":[{"type":"command","command":"say hi"}]}],
	  "Stop":[{"hooks":[{"type":"command","command":"echo user-stop"}]}]}}`), 0o600)

	for i := 0; i < 2; i++ { // idempotent
		if err := Install(p, 9876, "tok", false); err != nil {
			t.Fatal(err)
		}
	}
	m := read(t, p)
	hooks := m["hooks"].(map[string]any)
	notif := hooks["Notification"].([]any)
	if len(notif) != 2 {
		t.Fatalf("Notification groups = %d, want user + commander", len(notif))
	}
	ours := notif[1].(map[string]any)
	if _, has := ours["matcher"]; has {
		t.Fatalf("matcher = %v; every notification type must reach /notify", ours["matcher"])
	}
	cmd := ours["hooks"].([]any)[0].(map[string]any)["command"].(string)
	if !strings.Contains(cmd, Sentinel) || !strings.Contains(cmd, "9876") {
		t.Fatalf("command = %s", cmd)
	}
	if len(hooks["Stop"].([]any)) != 2 {
		t.Fatal("Stop groups wrong")
	}

	if err := Remove(p); err != nil {
		t.Fatal(err)
	}
	m = read(t, p)
	hooks = m["hooks"].(map[string]any)
	if n := hooks["Notification"].([]any); len(n) != 1 || strings.Contains(fmt.Sprint(n), Sentinel) {
		t.Fatalf("after remove Notification = %v", n)
	}
	if s := hooks["Stop"].([]any); len(s) != 1 {
		t.Fatalf("after remove Stop = %v", s)
	}
}

func TestInstallHTTPStop(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	if err := Install(p, 4242, "tok", true); err != nil {
		t.Fatal(err)
	}
	m, _ := load(p)
	hooks := m["hooks"].(map[string]any)
	stop := hooks["Stop"].([]any)
	h := stop[len(stop)-1].(map[string]any)["hooks"].([]any)[0].(map[string]any)
	if h["type"] != "http" || h["url"] != StopURL(4242, "tok") || h["timeout"] != float64(5) {
		t.Fatalf("stop hook = %v", h)
	}
	if !strings.HasPrefix(StopURL(4242, "tok"), "http://127.0.0.1:4242/") {
		t.Fatalf("StopURL = %s, want the IPv4 loopback the server listens on", StopURL(4242, "tok"))
	}
	hdr, _ := h["headers"].(map[string]any)
	env, _ := h["allowedEnvVars"].([]any)
	if hdr["X-Commander-Tag"] != "$COMMANDER_TAG" || len(env) != 1 || env[0] != "COMMANDER_TAG" {
		t.Fatalf("headers %v allowedEnvVars %v", h["headers"], h["allowedEnvVars"])
	}
	if !strings.Contains(StopURL(4242, "tok"), Sentinel) {
		t.Fatal("sentinel missing from URL")
	}
	// The other four events keep the curl command.
	n := hooks["Notification"].([]any)[0].(map[string]any)["hooks"].([]any)[0].(map[string]any)
	if n["type"] != "command" {
		t.Fatalf("Notification hook = %v", n)
	}
	// Re-install is idempotent; Remove sweeps the http group.
	if err := Install(p, 4242, "tok", true); err != nil {
		t.Fatal(err)
	}
	m, _ = load(p)
	if got := len(m["hooks"].(map[string]any)["Stop"].([]any)); got != 1 {
		t.Fatalf("Stop groups after reinstall = %d", got)
	}
	if err := Remove(p); err != nil {
		t.Fatal(err)
	}
	m, _ = load(p)
	if _, ok := m["hooks"]; ok {
		t.Fatalf("hooks left after Remove: %v", m["hooks"])
	}
}

func TestInstallCurlStopWhenNotReady(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	if err := Install(p, 4242, "tok", false); err != nil {
		t.Fatal(err)
	}
	m, _ := load(p)
	h := m["hooks"].(map[string]any)["Stop"].([]any)[0].(map[string]any)["hooks"].([]any)[0].(map[string]any)
	if h["type"] != "command" {
		t.Fatalf("stop hook = %v, want the curl command", h)
	}
}

func TestSwitchingShapesReplacesStop(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	_ = Install(p, 1, "a", true)
	_ = Install(p, 1, "a", false)
	m, _ := load(p)
	stop := m["hooks"].(map[string]any)["Stop"].([]any)
	if len(stop) != 1 {
		t.Fatalf("Stop groups = %d, want 1", len(stop))
	}
}
