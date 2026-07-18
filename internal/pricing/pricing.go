// Package pricing turns token counts into USD estimates.
package pricing

import "github.com/halalgami/CodingAgentCommander/internal/config"

// TurnInputCost is the USD cost to send `tokens` input tokens to model m
// (prices are per 1M tokens). On resume the cache is cold, so this is the
// honest worst-case first-turn input cost.
func TurnInputCost(tokens int, m config.Model) float64 {
	return float64(tokens) / 1_000_000 * m.InputPrice
}

// Band classifies a per-turn cost for color-coding: <$0.10 green,
// <$1.00 amber, else red.
func Band(costUSD float64) string {
	switch {
	case costUSD < 0.10:
		return "green"
	case costUSD < 1.00:
		return "amber"
	default:
		return "red"
	}
}

// ContextBand classifies context fullness against the 200k window for
// sessions where per-turn dollars are meaningless (subscription-billed native
// Anthropic): green while roomy, amber past half, red when compaction nears.
func ContextBand(tokens int) string {
	switch {
	case tokens < 100_000:
		return "green"
	case tokens < 160_000:
		return "amber"
	default:
		return "red"
	}
}
