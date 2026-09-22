package imagegen

import (
	"math"
	"strings"
	"testing"
	"time"
)

func TestEstimateUSD(t *testing.T) {
	fresh := PricedAt().Add(24 * time.Hour)
	cases := []struct {
		name  string
		model string
		n     int
		now   time.Time
		want  float64
		ok    bool
	}{
		// One input image plus six derived images at $0.08 each.
		{"seven images of a priced model", FalDefaultModel, 7, fresh, 0.56, true},
		{"one image", FalDefaultModel, 1, fresh, 0.08, true},
		{"zero images costs nothing", FalDefaultModel, 0, fresh, 0, true},
		{"unknown model is unpriced", "fal-ai/some-future-thing", 3, fresh, 0, false},
		{"negative count is refused", FalDefaultModel, -1, fresh, 0, false},
		// Past the gate, no number is offered at any count.
		{"stale table offers no figure", FalDefaultModel, 7,
			PricedAt().Add(PriceMaxAge + time.Hour), 0, false},
		{"the last day inside the window still quotes", FalDefaultModel, 7,
			PricedAt().Add(PriceMaxAge - time.Hour), 0.56, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := EstimateUSD(tc.model, tc.n, tc.now)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if math.Abs(got-tc.want) > 1e-9 {
				t.Errorf("EstimateUSD = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestStaleAt(t *testing.T) {
	if StaleAt(PricedAt()) {
		t.Error("the table is not stale on the day it was priced")
	}
	if !StaleAt(PricedAt().Add(PriceMaxAge + time.Nanosecond)) {
		t.Error("the table must be stale one instant past the window")
	}
}

func TestPriceUSD(t *testing.T) {
	p, ok := PriceUSD(FalDefaultModel)
	if !ok {
		t.Fatalf("%s is not in the price table", FalDefaultModel)
	}
	if p != 0.08 {
		t.Errorf("price = %v, want 0.08 per image", p)
	}
	if _, ok := PriceUSD("nope"); ok {
		t.Error("an unknown model must not resolve to a price")
	}
}

func TestEveryOfferedModelIsPriced(t *testing.T) {
	// A model offered in a picker with no price renders as free. Adding one to
	// Models() without a table entry is how that happens.
	//
	// Deliberately reached through Lookup rather than Providers(): the test
	// binary's registry also holds the fakes other test files register, and
	// whether they are present depends on test ordering.
	p, ok := Lookup("fal")
	if !ok {
		t.Fatal(`Lookup("fal") failed`)
	}
	for _, m := range p.Models() {
		if _, ok := PriceUSD(m.ID); !ok {
			t.Errorf("provider %q offers %q with no price-table entry", p.ID(), m.ID)
		}
	}
}

func TestPriceNoteCarriesTheDate(t *testing.T) {
	note := PriceNote()
	if !strings.Contains(note, PricedAt().Format("2006-01-02")) {
		t.Errorf("PriceNote() = %q, must state the date every figure is as of", note)
	}
}
