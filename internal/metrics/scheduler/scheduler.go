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

const (
	Quiet  = 10 * time.Second
	MinGap = 2 * time.Minute
)

type project struct {
	dirty              bool
	lastEdit, lastScan time.Time
	scanStart          time.Time
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
	p.dirty, p.lastEdit = true, s.now()
	s.mu.Unlock()
	s.poke()
}

// HeadMoved marks root for a scan as soon as MinGap allows.
func (s *Scheduler) HeadMoved(root string) {
	s.mu.Lock()
	p := s.get(root)
	p.dirty, p.lastEdit = true, s.now().Add(-Quiet)
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
		if p.dirty && now.Sub(p.lastEdit) >= Quiet && (p.lastScan.IsZero() || now.Sub(p.lastScan) >= MinGap) {
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

// Done ends a scan. A project edited after the scan started stays dirty.
func (s *Scheduler) Done(root string) {
	s.mu.Lock()
	p := s.get(root)
	p.lastScan = s.now()
	if !p.lastEdit.After(p.scanStart) {
		p.dirty = false
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
