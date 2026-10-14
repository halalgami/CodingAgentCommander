// Package parse turns Claude Code transcript lines into small typed events.
// Transcript records are undocumented internals: everything here is tolerant —
// unknown records are ignored, known records missing a field are marked
// Partial, and nothing panics (see FuzzClassify).
package parse

// MaxLine bounds one transcript line; longer lines are skipped, never buffered.
const MaxLine = 8 << 20

// ParserVersion is stored with cursors and session summaries. Bump it when a
// Classify change alters what is counted: ingest re-derives every session whose
// transcript still exists.
const ParserVersion = 5 // 2: usage once per message id; 3: progress counters; 4: re-read so ghost projects fold into their repo; 5: task notifications are not prompts

type Kind uint8

const (
	Prompt Kind = iota + 1
	Assistant
	ToolUse
	ToolResult
	TurnDuration
	Title
	AwaySummary
	Compact
	APIError
	Interrupted  // the user pressed Esc: "[Request interrupted by user…]"
	Notification // a background task reported back (origin.kind "task-notification"); not a prompt
)

type Usage struct{ In, Out, CacheRead, CacheWrite int64 }

// Event is one fact from a transcript line. Target and Command are for live
// display and commit/test detection only and must never be persisted.
type Event struct {
	Kind                               Kind
	At                                 int64 // ms since epoch; 0 if the line had no timestamp
	SessionID, Cwd, Branch, Entrypoint string
	UUID                               string
	Sidechain, Partial                 bool
	Model                              string
	MessageID                          string // assistant message.id; one message spans several lines
	Usage                              Usage
	HasUsage                           bool
	ToolID, ToolName, Target, FilePath string
	Command                            string
	AgentID, Status                    string // Notification: task id + status; ToolResult: from toolUseResult; Assistant: Status = stop_reason
	LinesAdded, LinesRemoved           int64
	IsError                            bool
	Ms                                 int64
	Text                               string
	PreTokens, PostTokens              int64
}
