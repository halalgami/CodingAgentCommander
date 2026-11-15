package quota

import (
	"bufio"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Store reads and writes <Dir>/<provider>-<yyyy-mm>.jsonl.
type Store struct{ Dir string }

func (s Store) file(provider string, t time.Time) string {
	return filepath.Join(s.Dir, provider+"-"+t.Local().Format("2006-01")+".jsonl")
}

// Append writes one sample to its month file, creating the directory.
func (s Store) Append(provider string, x Sample) error {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(x)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(s.file(provider, x.T), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, werr := f.Write(append(b, '\n'))
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	return werr
}

// monthOf parses the yyyy-mm suffix of a provider file name.
func monthOf(name, provider string) (time.Time, bool) {
	if !strings.HasPrefix(name, provider+"-") || !strings.HasSuffix(name, ".jsonl") {
		return time.Time{}, false
	}
	m, err := time.ParseInLocation("2006-01", strings.TrimSuffix(strings.TrimPrefix(name, provider+"-"), ".jsonl"), time.Local)
	return m, err == nil
}

// Read returns samples with from <= T < to, ascending. Unreadable lines are
// skipped; a missing directory is an empty history.
func (s Store) Read(provider string, from, to time.Time) ([]Sample, error) {
	ents, err := os.ReadDir(s.Dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Sample
	for _, e := range ents {
		m, ok := monthOf(e.Name(), provider)
		if !ok || !m.AddDate(0, 1, 0).After(from) || !m.Before(to) {
			continue
		}
		f, err := os.Open(filepath.Join(s.Dir, e.Name()))
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		for sc.Scan() {
			var x Sample
			if json.Unmarshal(sc.Bytes(), &x) != nil || x.T.Before(from) || !x.T.Before(to) {
				continue
			}
			out = append(out, x)
		}
		f.Close()
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].T.Before(out[j].T) })
	return out, nil
}

// Prune deletes month files whose month ended more than Retention ago.
func (s Store) Prune(now time.Time) error {
	ents, err := os.ReadDir(s.Dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range ents {
		const tail = len("-2006-01.jsonl")
		if len(e.Name()) <= tail {
			continue
		}
		m, ok := monthOf(e.Name(), e.Name()[:len(e.Name())-tail])
		if !ok {
			continue
		}
		if m.AddDate(0, 1, 0).Before(now.Add(-Retention)) {
			_ = os.Remove(filepath.Join(s.Dir, e.Name()))
		}
	}
	return nil
}
