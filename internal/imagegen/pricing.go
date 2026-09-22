package imagegen

import (
	"fmt"
	"time"
)

// pricedAt is the day the table below was last checked against the providers'
// published pricing. Every figure here is exactly as true as this date.
var pricedAt = time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC)

// PriceMaxAge is how long a figure may be shown. Past it, callers must stop
// rendering money entirely: an image count and a link to the provider's pricing
// page is a true statement, and a stale price stated confidently is not.
const PriceMaxAge = 90 * 24 * time.Hour

// priceTable is USD per successfully generated image, keyed by model id.
// Providers bill a flat rate per image and none reports a per-call charge, so
// this is the only cost figure that exists — the estimate is the actual, minus
// failures.
var priceTable = map[string]float64{
	FalDefaultModel: 0.08,
	// klein is billed per MEGAPIXEL, not per image, at $0.01/MP, and fal rounds
	// megapixels UP. $0.01 is therefore the per-image figure only because
	// downscaleToBudget caps every input at 1 Mpx before it is sent. It is NOT
	// klein's price in general, and this entry is WRONG the moment that cap
	// stops being enforced.
	//
	// That is not hypothetical: this comment previously asserted the downscale
	// while no such code existed — the spike did it in Python and the port
	// dropped it — so the app quoted $0.06 for a six-image run that could bill
	// $2.40. TestGenerateNeverSendsMoreThanOneMegapixel is what keeps the claim
	// honest; if it is ever deleted, delete this price with it.
	//
	// The quote is also an UNDER-estimate for this model in a way the others are
	// not: around 1 in 6 of its images comes back as a billed black frame, so
	// the real cost per USABLE image is nearer $0.012.
	"fal-ai/flux-2/klein/4b/edit": 0.01,
}

// PricedAt returns the day the table was last verified.
func PricedAt() time.Time { return pricedAt }

// PriceUSD returns the per-image price for a model id.
func PriceUSD(modelID string) (float64, bool) {
	p, ok := priceTable[modelID]
	return p, ok
}

// StaleAt reports whether the table is too old to quote from at the given time.
func StaleAt(now time.Time) bool { return now.Sub(pricedAt) > PriceMaxAge }

// EstimateUSD is the estimated charge for n images of modelID, as of now.
//
// ok is false when the model is unpriced, the count is nonsensical, or the
// table has aged out; in every one of those cases the caller must show a count
// rather than a number.
//
// This is a package function rather than a Provider method on purpose: pricing
// is a table a human maintains, not something a provider reports.
func EstimateUSD(modelID string, n int, now time.Time) (float64, bool) {
	if n < 0 || StaleAt(now) {
		return 0, false
	}
	p, ok := priceTable[modelID]
	if !ok {
		return 0, false
	}
	return p * float64(n), true
}

// PriceNote is the disclosure that must accompany any figure from this table.
func PriceNote() string {
	return fmt.Sprintf("prices as of %s", pricedAt.Format("2006-01-02"))
}
