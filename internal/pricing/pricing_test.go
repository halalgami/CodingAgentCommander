package pricing

import (
	"testing"

	"github.com/halalgami/CodingAgentCommander/internal/config"
)

func TestTurnInputCost(t *testing.T) {
	// 1,000,000 tokens at $15/1M = $15.00
	got := TurnInputCost(1_000_000, config.Model{InputPrice: 15})
	if got != 15.0 {
		t.Errorf("TurnInputCost = %v, want 15", got)
	}
}

func TestBand(t *testing.T) {
	cases := map[float64]string{0.05: "green", 0.30: "amber", 1.50: "red"}
	for cost, want := range cases {
		if got := Band(cost); got != want {
			t.Errorf("Band(%v) = %q, want %q", cost, got, want)
		}
	}
}
