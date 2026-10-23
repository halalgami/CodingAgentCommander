package scheduler

import (
	"testing"
	"time"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time      { return c.t }
func (c *clock) add(d time.Duration) { c.t = c.t.Add(d) }

func TestDebounceAndGap(t *testing.T) {
	c := &clock{time.Unix(1000, 0)}
	s := New(c.now)
	s.Touch("/r")
	if _, ok := s.Next(); ok {
		t.Fatal("scanned before Quiet elapsed")
	}
	c.add(Quiet)
	root, ok := s.Next()
	if !ok || root != "/r" {
		t.Fatal("not due after quiet")
	}
	s.Started("/r")
	s.Done("/r")
	s.Touch("/r")
	c.add(Quiet)
	if _, ok := s.Next(); ok {
		t.Fatal("rescanned within MinGap")
	}
	c.add(MinGap)
	if _, ok := s.Next(); !ok {
		t.Fatal("not due after MinGap")
	}
}

func TestEditDuringScanKeepsDirty(t *testing.T) {
	c := &clock{time.Unix(1000, 0)}
	s := New(c.now)
	s.Touch("/r")
	c.add(Quiet)
	s.Next()
	s.Started("/r")
	c.add(time.Second)
	s.Touch("/r") // edit lands while the scan runs
	c.add(time.Second)
	s.Done("/r")
	c.add(MinGap + Quiet)
	if root, ok := s.Next(); !ok || root != "/r" {
		t.Fatal("edit during scan was lost")
	}
}

func TestLiveBeforeBackgroundAndQueueOrder(t *testing.T) {
	c := &clock{time.Unix(1000, 0)}
	s := New(c.now)
	s.Queue("/bg1")
	s.Queue("/bg2")
	s.HeadMoved("/live")
	if root, _ := s.Next(); root != "/live" {
		t.Fatalf("got %s, live must go first", root)
	}
	s.Started("/live")
	s.Done("/live")
	if root, _ := s.Next(); root != "/bg1" {
		t.Fatalf("got %s, want FIFO /bg1", root)
	}
	s.Started("/bg1")
	if _, ok := s.Next(); ok {
		t.Fatal("second scan offered while one runs")
	}
	s.Done("/bg1")
	if root, _ := s.Next(); root != "/bg2" {
		t.Fatalf("got %s", root)
	}
}
