package pricing

import "testing"

func TestContextBandScalesWithTheWindow(t *testing.T) {
	cases := []struct {
		tokens, window int
		want           string
	}{
		// 200k window: the old fixed thresholds, now expressed as fractions.
		{0, 200_000, "green"},
		{99_999, 200_000, "green"},
		{100_000, 200_000, "amber"},
		{159_999, 200_000, "amber"},
		{160_000, 200_000, "red"},
		{210_000, 200_000, "red"},
		// 1M window: the same token counts are nowhere near full. Under the
		// old fixed 200k assumption every one of these read red.
		{160_000, 1_000_000, "green"},
		{499_999, 1_000_000, "green"},
		{500_000, 1_000_000, "amber"},
		{800_000, 1_000_000, "red"},
	}
	for _, c := range cases {
		if got := ContextBand(c.tokens, c.window); got != c.want {
			t.Errorf("ContextBand(%d, %d) = %q, want %q", c.tokens, c.window, got, c.want)
		}
	}
}

// A model with no reported window must not divide by zero or read as full.
func TestContextBandWithUnknownWindow(t *testing.T) {
	if got := ContextBand(50_000, 0); got != "green" {
		t.Errorf("ContextBand with a zero window = %q, want green", got)
	}
}
