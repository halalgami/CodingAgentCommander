// Package scheduler decides when to scan which project: debounced edit
// activity and HEAD moves for live projects first, then queued background
// work. One scan at a time.
package scheduler

import (
	"context"
	"sync"
	"time"

	"github.com/halalgami/CodingAgentCommander/internal/metrics/scan"
)

// A scan re-reads the whole tree and runs git three times, and what it feeds
// (90-day churn hotspots, one snapshot per day) moves slowly: wait for the
// agent to pause, and refresh a project at most every MinGap. An agent that
// never pauses for Quiet is scanned anyway once it has been dirty for MinGap.
const (
	Quiet  = time.Minute
	MinGap = 10 * time.Minute
)

type project struct {
	dirty              bool
	force              bool // ScanNow: skip Quiet and MinGap
	lastEdit, lastScan time.Time
	dirtySince         time.Time
	lastMark           time.Time // real time of the last mark; HeadMoved backdates the rest
	scanStart          time.Time
}

// mark makes p dirty at now, dated at (HeadMoved backdates), keeping the
// earliest moment it became so.
func (p *project) mark(now, at time.Time) {
	p.lastMark = now
	if !p.dirty || at.Before(p.dirtySince) {
		p.dirtySince = at
	}
	p.dirty = true
}

type Scheduler struct {
	mu       sync.Mutex
	now      func() time.Time
	projects map[string]*project
	queue    []string
	running  string
	wake     chan struct{}
}

func New(now func() time.Time) *Scheduler {
	return &Scheduler{now: now, projects: map[string]*project{}, wake: make(chan struct{}, 1)}
}

func (s *Scheduler) get(root string) *project {
	p := s.projects[root]
	if p == nil {
		p = &project{}
		s.projects[root] = p
	}
	return p
}

func (s *Scheduler) poke() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Touch records Claude edit activity in root.
func (s *Scheduler) Touch(root string) {
	s.mu.Lock()
	p := s.get(root)
	p.mark(s.now(), s.now())
	p.lastEdit = s.now()
	s.mu.Unlock()
	s.poke()
}

// HeadMoved marks root for a scan as soon as MinGap allows, without waiting
// for edits to go quiet.
func (s *Scheduler) HeadMoved(root string) {
	s.mu.Lock()
	p := s.get(root)
	p.mark(s.now(), s.now().Add(-MinGap))
	if !p.lastEdit.After(s.now().Add(-Quiet)) {
		p.lastEdit = s.now().Add(-Quiet)
	}
	s.mu.Unlock()
	s.poke()
}

// ScanNow marks root for a scan at the next free slot (the user asked).
func (s *Scheduler) ScanNow(root string) {
	s.mu.Lock()
	p := s.get(root)
	p.mark(s.now(), s.now())
	p.force = true
	s.mu.Unlock()
	s.poke()
}

// Queue requests a background scan of root (first pass, weekly).
func (s *Scheduler) Queue(root string) {
	s.mu.Lock()
	for _, q := range s.queue {
		if q == root {
			s.mu.Unlock()
			return
		}
	}
	s.queue = append(s.queue, root)
	s.mu.Unlock()
	s.poke()
}

// Next returns the project due now, live work first. Nothing while a scan runs.
func (s *Scheduler) Next() (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running != "" {
		return "", false
	}
	now := s.now()
	best, bestEdit := "", time.Time{}
	for root, p := range s.projects {
		settled := now.Sub(p.lastEdit) >= Quiet || now.Sub(p.dirtySince) >= MinGap
		if p.force || p.dirty && settled && (p.lastScan.IsZero() || now.Sub(p.lastScan) >= MinGap) {
			if best == "" || p.lastEdit.Before(bestEdit) {
				best, bestEdit = root, p.lastEdit
			}
		}
	}
	if best != "" {
		return best, true
	}
	if len(s.queue) > 0 {
		return s.queue[0], true
	}
	return "", false
}

func (s *Scheduler) Started(root string) {
	s.mu.Lock()
	s.running = root
	s.get(root).scanStart = s.now()
	for i, q := range s.queue {
		if q == root {
			s.queue = append(s.queue[:i], s.queue[i+1:]...)
			break
		}
	}
	s.mu.Unlock()
}

// Done ends a scan. A project marked after the scan started stays dirty.
func (s *Scheduler) Done(root string) {
	s.mu.Lock()
	p := s.get(root)
	p.lastScan = s.now()
	p.force = false
	if !p.lastMark.After(p.scanStart) {
		p.dirty = false
	} else if p.dirtySince.Before(p.scanStart) {
		p.dirtySince = p.scanStart // what predates the scan was scanned
	}
	s.running = ""
	s.mu.Unlock()
	s.poke()
}

// Run executes due scans until ctx ends.
func (s *Scheduler) Run(ctx context.Context, scanFn func(context.Context, string) (scan.Result, error), onResult func(string, scan.Result, error)) {
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		case <-s.wake:
		}
		root, ok := s.Next()
		if !ok {
			continue
		}
		s.Started(root)
		r, err := scanFn(ctx, root)
		s.Done(root)
		onResult(root, r, err)
	}
}
