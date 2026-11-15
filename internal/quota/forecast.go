package quota

import "time"

// Forecast says when a window reaches 100 % at the current pace.
type Forecast struct {
	HitAt time.Time `json:"hitAt"`
	OK    bool      `json:"ok"`
	Note  string    `json:"note"` // why not OK: "not enough samples" | "not rising" | "resets first"
}

// ForecastOf fits a line to the last 24 h of samples of window key that
// belong to the newest window (same ResetsAt) and extrapolates to 100 %.
func ForecastOf(samples []Sample, key string, now time.Time) Forecast {
	type pt struct {
		t time.Time
		u float64
		r string
	}
	var pts []pt
	for _, s := range samples {
		for _, w := range s.Windows {
			if w.Key == key {
				pts = append(pts, pt{s.T, w.Util, w.ResetsAt})
			}
		}
	}
	if len(pts) == 0 {
		return Forecast{Note: "not enough samples"}
	}
	last := pts[len(pts)-1]
	// Walk back from the newest sample while it is the same window: resets_at
	// within 2 min (the endpoint's value may jitter) and no drop in util
	// (a drop means the window reset). Only the last 24 h count.
	use := []pt{last}
	for i := len(pts) - 2; i >= 0; i-- {
		p := pts[i]
		if !sameWindow(p.r, last.r) || p.u > use[0].u+1 || p.t.Before(now.Add(-24*time.Hour)) {
			break
		}
		use = append([]pt{p}, use...)
	}
	if last.u >= 100 {
		return Forecast{HitAt: last.t, OK: true}
	}
	if len(use) < 2 || use[len(use)-1].t.Sub(use[0].t) < 10*time.Minute {
		return Forecast{Note: "not enough samples"}
	}
	t0 := use[0].t
	var n, sx, sy, sxx, sxy float64
	for _, p := range use {
		x := p.t.Sub(t0).Hours()
		n++
		sx += x
		sy += p.u
		sxx += x * x
		sxy += x * p.u
	}
	den := n*sxx - sx*sx
	if den == 0 {
		return Forecast{Note: "not enough samples"}
	}
	slope := (n*sxy - sx*sy) / den
	if slope <= 1e-9 {
		return Forecast{Note: "not rising"}
	}
	icpt := (sy - slope*sx) / n
	xl := last.t.Sub(t0).Hours()
	fit := icpt + slope*xl
	hit := last.t.Add(time.Duration((100 - fit) / slope * float64(time.Hour)))
	if hit.Before(last.t) {
		hit = last.t // the fit is already past 100: the limit is reached
	}
	if r, err := time.Parse(time.RFC3339, last.r); err == nil && hit.After(r) {
		return Forecast{Note: "resets first"}
	}
	return Forecast{HitAt: hit, OK: true}
}

// sameWindow: two resets_at values name the same window when both parse and
// lie within 2 minutes of each other; otherwise they must be equal strings.
func sameWindow(a, b string) bool {
	ta, ea := time.Parse(time.RFC3339, a)
	tb, eb := time.Parse(time.RFC3339, b)
	if ea != nil || eb != nil {
		return a == b
	}
	return ta.Sub(tb).Abs() <= 2*time.Minute
}
