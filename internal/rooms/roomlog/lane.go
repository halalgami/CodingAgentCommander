package roomlog

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/halalgami/CodingAgentCommander/internal/rooms/canon"
)

const (
	maxLamport     = 1 << 53
	maxLamportJump = 1 << 32
)

var (
	ErrGap       = errors.New("lane gap")
	ErrDuplicate = errors.New("duplicate entry")
	ErrTooLarge  = errors.New("entry too large")
	ErrLanes     = errors.New("too many lanes")
)

// Lanes is one room's lanes on disk: <Dir>/<device>.jsonl, append-only.
type Lanes struct {
	Dir  string
	Room string
	// Admit decides whether a new lane may open with its first entry (set by
	// the service: a device.cert of the owner or a current member). nil = any.
	Admit func(first Entry) bool

	mu      sync.Mutex
	loaded  bool
	lanes   map[string][]Entry
	lamport int64
}

func (l *Lanes) loadLocked() {
	if l.loaded {
		return
	}
	l.loaded = true
	l.lanes = map[string][]Entry{}
	ents, _ := os.ReadDir(l.Dir) // not Glob: a '[' in the home path is a pattern
	for _, de := range ents {
		if de.IsDir() || filepath.Ext(de.Name()) != ".jsonl" {
			continue
		}
		f := filepath.Join(l.Dir, de.Name())
		body, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		good := 0
		for good < len(body) {
			nl := bytes.IndexByte(body[good:], '\n')
			if nl < 0 {
				break // a torn last line (no newline): drop it below
			}
			var e Entry
			if json.Unmarshal(body[good:good+nl], &e) != nil || e.Seq != int64(len(l.lanes[e.Device]))+1 {
				break
			}
			l.lanes[e.Device] = append(l.lanes[e.Device], e)
			l.lamport = max(l.lamport, e.Lamport)
			good += nl + 1
		}
		if good < len(body) {
			// Cut the file back to its last good entry so the next append
			// starts on a clean line and nothing after it is lost on reload.
			os.Truncate(f, int64(good))
		}
	}
}

func (l *Lanes) headLocked(dev string) int64 { return int64(len(l.lanes[dev])) }

// Append verifies and stores one entry. ErrDuplicate is harmless.
func (l *Lanes) Append(e Entry) error {
	if e.Room != l.Room {
		return fmt.Errorf("entry for room %q in room %q", e.Room, l.Room)
	}
	b, err := canon.Marshal(e)
	if err != nil {
		return err
	}
	if len(b) > MaxEntry {
		return ErrTooLarge
	}
	if strings.ContainsAny(e.Device, `/\.`) || e.Device == "" {
		return errors.New("bad device id")
	}
	if err := Verify(e); err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.loadLocked()
	head := l.headLocked(e.Device)
	switch {
	case e.Seq <= head:
		return ErrDuplicate
	case e.Seq != head+1:
		return fmt.Errorf("%w: have %d, got %d", ErrGap, head, e.Seq)
	case head == 0 && len(l.lanes) >= MaxLanes:
		return ErrLanes
	case e.Lamport < 1 || (head > 0 && e.Lamport <= l.lanes[e.Device][head-1].Lamport):
		return fmt.Errorf("lamport %d must rise within a lane", e.Lamport)
	case e.Lamport > maxLamport || e.Lamport > l.lamport+maxLamportJump:
		// One poisoned value would make every later write overflow (review C1).
		return fmt.Errorf("lamport %d is implausibly far ahead", e.Lamport)
	case head == 0 && l.Admit != nil && !l.Admit(e):
		return errors.New("a new lane must open with a member's device certificate")
	}
	if err := os.MkdirAll(l.Dir, 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(l.Dir, e.Device+".jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	l.lanes[e.Device] = append(l.lanes[e.Device], e)
	l.lamport = max(l.lamport, e.Lamport)
	return nil
}

func (l *Lanes) Heads() map[string]int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.loadLocked()
	h := make(map[string]int64, len(l.lanes))
	for d, es := range l.lanes {
		h[d] = int64(len(es))
	}
	return h
}

// Since returns lane entries after seq, up to maxBytes of encoded size (at least one).
func (l *Lanes) Since(dev string, after int64, maxBytes int) []Entry {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.loadLocked()
	es := l.lanes[dev]
	if after < 0 || after >= int64(len(es)) {
		return nil
	}
	var out []Entry
	size := 0
	for _, e := range es[after:] {
		b, _ := json.Marshal(e)
		if len(out) > 0 && size+len(b) > maxBytes {
			break
		}
		size += len(b)
		out = append(out, e)
	}
	return out
}

// All returns every entry in display order: Lamport, then device id.
func (l *Lanes) All() []Entry {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.loadLocked()
	var out []Entry
	for _, es := range l.lanes {
		out = append(out, es...)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Lamport != out[j].Lamport {
			return out[i].Lamport < out[j].Lamport
		}
		if out[i].Device != out[j].Device {
			return out[i].Device < out[j].Device
		}
		return out[i].Seq < out[j].Seq
	})
	return out
}
