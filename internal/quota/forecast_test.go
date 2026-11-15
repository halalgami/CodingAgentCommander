package quota

import (
	"testing"
	"time"
)

func series(now time.Time, resets time.Time, from, to float64, span time.Duration) []Sample {
	var out []Sample
	n := int(span / (10 * time.Minute))
	for i := 0; i <= n; i++ {
		t := now.Add(-span + time.Duration(i)*10*time.Minute)
		u := from + (to-from)*float64(i)/float64(n)
		out = append(out, Sample{T: t, Windows: []Window{{Key: "k", Util: u, ResetsAt: resets.Format(time.RFC3339)}}})
	}
	return out
}

var now0 = time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)

func TestForecastResetsFirst(t *testing.T) {
	f := ForecastOf(series(now0, now0.Add(4*time.Hour), 10, 30, 2*time.Hour), "k", now0)
	if f.OK || f.Note != "resets first" {
		t.Fatalf("%+v", f)
	}
}

func TestForecastHitsBeforeReset(t *testing.T) {
	f := ForecastOf(series(now0, now0.Add(6*24*time.Hour), 20, 50, 24*time.Hour), "k", now0)
	want := now0.Add(40 * time.Hour)
	if !f.OK || f.HitAt.Sub(want).Abs() > 5*time.Minute {
		t.Fatalf("%+v want %v", f, want)
	}
}

func TestForecastIgnoresOlderWindow(t *testing.T) {
	old := series(now0.Add(-3*time.Hour), now0.Add(-2*time.Hour), 80, 90, time.Hour)
	cur := series(now0, now0.Add(6*24*time.Hour), 5, 10, time.Hour)
	f := ForecastOf(append(old, cur...), "k", now0)
	// slope 5/h from the current window only: 10 → 100 in 18 h
	if !f.OK || f.HitAt.Sub(now0.Add(18*time.Hour)).Abs() > 5*time.Minute {
		t.Fatalf("%+v", f)
	}
}

func TestForecastIgnoresOlderThan24h(t *testing.T) {
	resets := now0.Add(6 * 24 * time.Hour)
	old := series(now0.Add(-30*time.Hour), resets, 0, 0, time.Hour) // flat, too old
	cur := series(now0, resets, 10, 20, 2*time.Hour)                // 5/h
	f := ForecastOf(append(old, cur...), "k", now0)
	if !f.OK || f.HitAt.Sub(now0.Add(16*time.Hour)).Abs() > 5*time.Minute {
		t.Fatalf("%+v", f)
	}
}

func TestForecastNeedsTwoPointsAnd10Min(t *testing.T) {
	r := now0.Add(time.Hour).Format(time.RFC3339)
	one := []Sample{{T: now0, Windows: []Window{{Key: "k", Util: 10, ResetsAt: r}}}}
	if f := ForecastOf(one, "k", now0); f.OK || f.Note != "not enough samples" {
		t.Fatalf("%+v", f)
	}
	two := append(one[:0:0], Sample{T: now0.Add(-5 * time.Minute), Windows: []Window{{Key: "k", Util: 5, ResetsAt: r}}}, one[0])
	if f := ForecastOf(two, "k", now0); f.OK || f.Note != "not enough samples" {
		t.Fatalf("%+v", f)
	}
}

func TestForecastFlat(t *testing.T) {
	f := ForecastOf(series(now0, now0.Add(6*24*time.Hour), 40, 40, 2*time.Hour), "k", now0)
	if f.OK || f.Note != "not rising" {
		t.Fatalf("%+v", f)
	}
}

func TestForecastAlreadyAtLimit(t *testing.T) {
	f := ForecastOf(series(now0, now0.Add(time.Hour), 90, 100, time.Hour), "k", now0)
	if !f.OK || !f.HitAt.Equal(now0) {
		t.Fatalf("%+v", f)
	}
}

func TestForecastToleratesResetsAtJitter(t *testing.T) {
	// Review #1: resets_at may carry different fractional seconds per call.
	resets := now0.Add(6 * 24 * time.Hour)
	var xs []Sample
	for i := 0; i <= 12; i++ {
		r := resets.Add(time.Duration(i) * 137 * time.Millisecond).Format(time.RFC3339Nano) // every value differs
		xs = append(xs, Sample{T: now0.Add(-2*time.Hour + time.Duration(i)*10*time.Minute), Windows: []Window{{Key: "k", Util: 10 + float64(i)*10/12, ResetsAt: r}}})
	}
	f := ForecastOf(xs, "k", now0) // 5/h from 20 → 100 in 16 h
	if !f.OK || f.HitAt.Sub(now0.Add(16*time.Hour)).Abs() > 5*time.Minute {
		t.Fatalf("%+v", f)
	}
}

func TestForecastUtilDropStartsNewWindow(t *testing.T) {
	// A drop in util is a reset even when resets_at did not change.
	resets := now0.Add(6 * 24 * time.Hour)
	old := series(now0.Add(-90*time.Minute), resets, 80, 90, 30*time.Minute)
	cur := series(now0, resets, 5, 10, time.Hour)
	f := ForecastOf(append(old, cur...), "k", now0)
	if !f.OK || f.HitAt.Sub(now0.Add(18*time.Hour)).Abs() > 5*time.Minute {
		t.Fatalf("%+v", f)
	}
}

func TestForecastNeverBeforeLastSample(t *testing.T) {
	// Review #3: a fit above 100 must not put the hit in the past of the data.
	r := now0.Add(24 * time.Hour).Format(time.RFC3339)
	xs := []Sample{
		{T: now0.Add(-30 * time.Minute), Windows: []Window{{Key: "k", Util: 50, ResetsAt: r}}},
		{T: now0.Add(-20 * time.Minute), Windows: []Window{{Key: "k", Util: 99, ResetsAt: r}}},
		{T: now0.Add(-10 * time.Minute), Windows: []Window{{Key: "k", Util: 99.5, ResetsAt: r}}},
	}
	f := ForecastOf(xs, "k", now0)
	if !f.OK || f.HitAt.Before(now0.Add(-10*time.Minute)) {
		t.Fatalf("%+v", f)
	}
}
