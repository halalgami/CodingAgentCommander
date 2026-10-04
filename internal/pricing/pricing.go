// Package pricing bands a session's context fullness for the card meter.
//
// The name is a leftover. It used to turn token counts into USD, but no
// provider in the catalog carries a rate any more: the native Anthropic table
// was hand-maintained, went stale between releases, and quoted per-turn dollars
// for sessions billed by subscription, while Bedrock, Zen, and Ollama never set
// one at all. Context fullness is the signal that was actually driving every
// meter, so it is the only one left.
package pricing

// ContextBand classifies context fullness against the model's own window:
// green while roomy, amber past half, red when compaction nears.
//
// The thresholds are fractions rather than token counts because the window is
// no longer one number. Fixing them at 100k/160k assumed a 200k window, which
// turned red at 16% full on the 1M-context models that are now the whole
// catalog — a permanently alarmed meter that said nothing.
func ContextBand(tokens, window int) string {
	if window <= 0 {
		return "green"
	}
	switch frac := float64(tokens) / float64(window); {
	case frac < 0.5:
		return "green"
	case frac < 0.8:
		return "amber"
	default:
		return "red"
	}
}
