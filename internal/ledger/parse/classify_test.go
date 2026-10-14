package parse

import (
	"strings"
	"testing"
	"unicode/utf8"
)

const (
	lineAssistantTools = `{"type":"assistant","timestamp":"2026-10-09T10:00:00.000Z","sessionId":"s1","cwd":"/repo","gitBranch":"main","entrypoint":"cli","uuid":"u1","message":{"id":"msg_01","model":"claude-opus-5-5","content":[{"type":"thinking","thinking":"hm"},{"type":"tool_use","id":"t1","name":"Edit","input":{"file_path":"/repo/app.go","old_string":"a\nb","new_string":"a\nb\nc"}},{"type":"tool_use","id":"t2","name":"Bash","input":{"command":"go test ./...\n  -run X"}}],"usage":{"input_tokens":10,"output_tokens":5,"cache_read_input_tokens":100,"cache_creation_input_tokens":20}}}`
	lineToolResults    = `{"type":"user","timestamp":"2026-10-09T10:00:01.000Z","sessionId":"s1","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"},{"type":"tool_result","tool_use_id":"t2","is_error":true,"content":"FAIL"}]}}`
	linePrompt         = `{"type":"user","timestamp":"2026-10-09T09:59:00.000Z","sessionId":"s1","cwd":"/repo","message":{"role":"user","content":"fix the bug"}}`
	linePromptArray    = `{"type":"user","timestamp":"2026-10-09T09:59:30.000Z","sessionId":"s1","message":{"role":"user","content":[{"type":"text","text":"look"},{"type":"image","source":{}}]}}`
	lineMeta           = `{"type":"user","isMeta":true,"sessionId":"s1","message":{"role":"user","content":"<local-command-caveat>"}}`
	lineTurn           = `{"type":"system","subtype":"turn_duration","durationMs":195035,"timestamp":"2026-10-09T10:03:00.000Z","sessionId":"s1","uuid":"td1"}`
	lineTurnNoMs       = `{"type":"system","subtype":"turn_duration","timestamp":"2026-10-09T10:03:00.000Z","sessionId":"s1","uuid":"td2"}`
	lineTitle          = `{"type":"ai-title","aiTitle":"Add GIF support for companion panel","sessionId":"s1"}`
	lineAway           = `{"type":"system","subtype":"away_summary","content":"I'm rebuilding the app.","timestamp":"2026-10-09T10:04:00.000Z","sessionId":"s1"}`
	lineCompact        = `{"type":"system","subtype":"compact_boundary","compactMetadata":{"trigger":"auto","preTokens":1000072,"postTokens":25444},"timestamp":"2026-10-09T10:05:00.000Z","sessionId":"s1"}`
	lineAPIError       = `{"type":"assistant","isApiErrorMessage":true,"timestamp":"2026-10-09T10:06:00.000Z","sessionId":"s1","message":{"model":"<synthetic>","content":[{"type":"text","text":"API Error: 529"}]}}`
	lineUnknown        = `{"type":"queue-operation","operation":"enqueue","sessionId":"s1"}`
	lineBroken         = `{"type":"assistant","message":`
)

func kinds(evs []Event) []Kind {
	out := make([]Kind, len(evs))
	for i, e := range evs {
		out[i] = e.Kind
	}
	return out
}

func TestClassifyAssistantWithTools(t *testing.T) {
	evs := Classify([]byte(lineAssistantTools))
	if len(evs) != 3 || evs[0].Kind != Assistant || evs[1].Kind != ToolUse || evs[2].Kind != ToolUse {
		t.Fatalf("kinds = %v", kinds(evs))
	}
	a := evs[0]
	if a.MessageID != "msg_01" {
		t.Fatalf("message id = %q", a.MessageID)
	}
	if a.Model != "claude-opus-5-5" || !a.HasUsage || a.Usage != (Usage{In: 10, Out: 5, CacheRead: 100, CacheWrite: 20}) {
		t.Fatalf("assistant = %+v", a)
	}
	if a.SessionID != "s1" || a.Cwd != "/repo" || a.Branch != "main" || a.Entrypoint != "cli" || a.At != 1791540000000 {
		t.Fatalf("base fields = %+v", a)
	}
	edit := evs[1]
	if edit.ToolID != "t1" || edit.ToolName != "Edit" || edit.Target != "app.go" || edit.FilePath != "/repo/app.go" ||
		edit.LinesAdded != 3 || edit.LinesRemoved != 2 {
		t.Fatalf("edit = %+v", edit)
	}
	bash := evs[2]
	if bash.Target != "go test ./... -run X" || !strings.HasPrefix(bash.Command, "go test") {
		t.Fatalf("bash = %+v", bash)
	}
}

func TestClassifyUserLines(t *testing.T) {
	if k := kinds(Classify([]byte(linePrompt))); len(k) != 1 || k[0] != Prompt {
		t.Fatalf("prompt: %v", k)
	}
	if k := kinds(Classify([]byte(linePromptArray))); len(k) != 1 || k[0] != Prompt {
		t.Fatalf("array prompt: %v", k)
	}
	if evs := Classify([]byte(lineMeta)); len(evs) != 0 {
		t.Fatalf("meta must be ignored: %v", kinds(evs))
	}
	evs := Classify([]byte(lineToolResults))
	if len(evs) != 2 || evs[0].ToolID != "t1" || evs[0].IsError || evs[1].ToolID != "t2" || !evs[1].IsError {
		t.Fatalf("tool results = %+v", evs)
	}
}

func TestClassifySystemAndTitle(t *testing.T) {
	td := Classify([]byte(lineTurn))
	if len(td) != 1 || td[0].Kind != TurnDuration || td[0].Ms != 195035 || td[0].UUID != "td1" || td[0].Partial {
		t.Fatalf("turn = %+v", td)
	}
	if p := Classify([]byte(lineTurnNoMs)); len(p) != 1 || !p[0].Partial {
		t.Fatalf("turn without durationMs must be Partial: %+v", p)
	}
	if ti := Classify([]byte(lineTitle)); len(ti) != 1 || ti[0].Kind != Title || ti[0].Text != "Add GIF support for companion panel" {
		t.Fatalf("title = %+v", ti)
	}
	if aw := Classify([]byte(lineAway)); len(aw) != 1 || aw[0].Kind != AwaySummary || aw[0].Text != "I'm rebuilding the app." {
		t.Fatalf("away = %+v", aw)
	}
	if c := Classify([]byte(lineCompact)); len(c) != 1 || c[0].Kind != Compact || c[0].PreTokens != 1000072 || c[0].PostTokens != 25444 {
		t.Fatalf("compact = %+v", c)
	}
	if e := Classify([]byte(lineAPIError)); len(e) != 2 || e[1].Kind != APIError {
		t.Fatalf("api error = %v", kinds(e))
	}
}

func TestClassifyIgnoresUnknownAndBroken(t *testing.T) {
	for _, l := range []string{lineUnknown, lineBroken, "", "not json"} {
		if evs := Classify([]byte(l)); len(evs) != 0 {
			t.Fatalf("%q produced %v", l, kinds(evs))
		}
	}
}

func TestClipKeepsValidUTF8AndOneLine(t *testing.T) {
	in := "مرحبا 👋 " + strings.Repeat("ث", 500) + "\n<img src=x>"
	out := Clip(oneLine(in), 60)
	if !utf8.ValidString(out) {
		t.Fatal("clip produced invalid UTF-8")
	}
	if n := utf8.RuneCountInString(out); n > 60 {
		t.Fatalf("clip length %d runes", n)
	}
	if strings.ContainsAny(out, "\n\r\t") {
		t.Fatal("target must be single-line")
	}
}

func FuzzClassify(f *testing.F) {
	for _, l := range []string{lineAssistantTools, lineToolResults, linePrompt, lineTurn, lineTitle, lineAway, lineCompact, lineAPIError, lineBroken} {
		f.Add([]byte(l))
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		for _, e := range Classify(b) {
			if !utf8.ValidString(e.Text) || !utf8.ValidString(e.Target) {
				t.Fatalf("invalid UTF-8 in %+v", e)
			}
		}
	})
}

func TestClassifyInterruptIsNotAPrompt(t *testing.T) {
	for _, l := range []string{
		`{"type":"user","timestamp":"2026-10-11T15:00:00.000Z","sessionId":"s1","message":{"role":"user","content":[{"type":"text","text":"[Request interrupted by user]"}]}}`,
		`{"type":"user","timestamp":"2026-10-11T15:00:00.000Z","sessionId":"s1","message":{"role":"user","content":[{"type":"text","text":"[Request interrupted by user for tool use]"}]}}`,
		`{"type":"user","timestamp":"2026-10-11T15:00:00.000Z","sessionId":"s1","message":{"role":"user","content":"[Request interrupted by user]"}}`,
	} {
		if k := kinds(Classify([]byte(l))); len(k) != 1 || k[0] != Interrupted {
			t.Fatalf("%s → %v, want [Interrupted]", l, k)
		}
	}
}
