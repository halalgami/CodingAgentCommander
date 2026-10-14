// Package scan measures one repository's working tree: per-file counts,
// 90-day churn and hotspots. All git access goes through safegit; only paths
// git returns are read, and only if their real path stays inside the repo.
package scan

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/halalgami/CodingAgentCommander/internal/metrics/count"
	"github.com/halalgami/CodingAgentCommander/internal/safegit"
)

type FileMetrics struct {
	Lang       string  `json:"lang"`
	Code       int     `json:"code"`
	Comment    int     `json:"comment"`
	Blank      int     `json:"blank"`
	Complexity int     `json:"complexity"`
	Todos      int     `json:"todos,omitempty"`
	Churn90    int     `json:"churn90,omitempty"`
	Hotspot    float64 `json:"hotspot,omitempty"`
	Flagged    bool    `json:"flagged,omitempty"`
	Test       bool    `json:"test,omitempty"`
}

type Snapshot struct {
	Day        string         `json:"day"`
	Files      int            `json:"files"`
	Code       int            `json:"code"`
	Complexity int            `json:"complexity"`
	Tests      int            `json:"tests"`
	Todos      int            `json:"todos"`
	Langs      map[string]int `json:"langs"`
	TestRatio  float64        `json:"testRatio"`
	Head       string         `json:"head"`
	Partial    bool           `json:"partial,omitempty"`
}

type Result struct {
	Head     string
	Files    map[string]FileMetrics
	Snapshot Snapshot
	Partial  bool
	Took     time.Duration
}

type Limits struct {
	Wall           time.Duration
	Files          int
	Bytes, FileMax int64
}

// gitTimeout bounds ls-files and log inside the scan's wall budget.
const gitTimeout = 15 * time.Second

var DefaultLimits = Limits{Wall: 20 * time.Second, Files: 20000, Bytes: 200 << 20, FileMax: 1 << 20}

func realRoot(root string) string {
	if r, err := filepath.EvalSymlinks(root); err == nil {
		return r
	}
	return filepath.Clean(root)
}

// inside reports whether abs, with symlinks resolved, is a regular file inside root.
func inside(rootReal, abs string) (os.FileInfo, bool) {
	fi, err := os.Lstat(abs)
	if err != nil || !fi.Mode().IsRegular() {
		return nil, false
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, false
	}
	rel, err := filepath.Rel(rootReal, real)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, false
	}
	return fi, true
}

// Repo scans root's working tree (tracked + untracked-not-ignored files).
func Repo(ctx context.Context, root string, lim Limits, now func() time.Time) (Result, error) {
	start := time.Now()
	list, err := safegit.RunTimeout(ctx, gitTimeout, root, "ls-files", "-co", "--exclude-standard", "-z")
	truncated := errors.Is(err, safegit.ErrTruncated)
	if err != nil && !truncated {
		return Result{}, err
	}
	head, _ := safegit.Run(ctx, root, "rev-parse", "HEAD")
	res := Result{Head: head, Files: map[string]FileMetrics{}, Partial: truncated}
	rootReal := realRoot(root)
	var read int64
	for _, rel := range strings.Split(list, "\x00") {
		if rel == "" {
			continue
		}
		if len(res.Files) >= lim.Files || read >= lim.Bytes || time.Since(start) > lim.Wall || ctx.Err() != nil {
			res.Partial = true
			break
		}
		abs := filepath.Join(root, filepath.FromSlash(rel))
		fi, ok := inside(rootReal, abs)
		if !ok || fi.Size() > lim.FileMax {
			continue
		}
		b, err := os.ReadFile(abs)
		if err != nil {
			continue
		}
		read += int64(len(b))
		st := count.Count(rel, b)
		if st.Excluded {
			continue
		}
		res.Files[rel] = FileMetrics{Lang: st.Lang, Code: st.Code, Comment: st.Comment, Blank: st.Blank,
			Complexity: st.Complexity, Todos: st.Todos, Test: st.Test}
		runtime.Gosched()
	}
	if !churn(ctx, root, res.Files) {
		res.Partial = true // churn unknown or cut short: "no hotspots" would be a lie
	}
	hotspots(res.Files)
	res.Snapshot = snapshot(res, now)
	res.Took = time.Since(start)
	return res, nil
}

// churn adds 90-day churn to files; false when the log failed or was cut off.
func churn(ctx context.Context, root string, files map[string]FileMetrics) bool {
	out, err := safegit.RunTimeout(ctx, gitTimeout, root, "log", "--since=90.days", "--numstat", "--format=")
	complete := err == nil
	if err != nil && !errors.Is(err, safegit.ErrTruncated) {
		return false
	}
	for _, line := range strings.Split(out, "\n") {
		f := strings.SplitN(line, "\t", 3)
		if len(f) != 3 {
			continue
		}
		a, err1 := strconv.Atoi(f[0])
		d, err2 := strconv.Atoi(f[1])
		if err1 != nil || err2 != nil { // binary files report "-"
			continue
		}
		p := renamedTo(f[2])
		if fm, ok := files[p]; ok {
			fm.Churn90 += a + d
			files[p] = fm
		}
	}
	return complete
}

// renamedTo maps git's numstat rename forms to the new path.
func renamedTo(p string) string {
	if i := strings.Index(p, "{"); i >= 0 {
		if j := strings.Index(p[i:], "}"); j >= 0 {
			inner := p[i+1 : i+j]
			if k := strings.Index(inner, " => "); k >= 0 {
				p = p[:i] + inner[k+4:] + p[i+j+1:]
				return strings.ReplaceAll(p, "//", "/")
			}
		}
	}
	if k := strings.Index(p, " => "); k >= 0 {
		return p[k+4:]
	}
	return p
}

func hotspots(files map[string]FileMetrics) {
	maxC, maxX := 0, 0
	for _, f := range files {
		if f.Churn90 > maxC {
			maxC = f.Churn90
		}
		if f.Complexity > maxX {
			maxX = f.Complexity
		}
	}
	if maxC == 0 || maxX == 0 {
		return
	}
	type kv struct {
		p string
		s float64
	}
	var ranked []kv
	for p, f := range files {
		f.Hotspot = float64(f.Churn90) / float64(maxC) * float64(f.Complexity) / float64(maxX)
		files[p] = f
		if f.Hotspot > 0 {
			ranked = append(ranked, kv{p, f.Hotspot})
		}
	}
	sort.Slice(ranked, func(i, j int) bool {
		return ranked[i].s > ranked[j].s || ranked[i].s == ranked[j].s && ranked[i].p < ranked[j].p
	})
	n := int(math.Ceil(0.05 * float64(len(files))))
	if n < 1 {
		n = 1
	}
	for i := 0; i < n && i < len(ranked); i++ {
		f := files[ranked[i].p]
		f.Flagged = true
		files[ranked[i].p] = f
	}
}

func snapshot(r Result, now func() time.Time) Snapshot {
	s := Snapshot{Day: now().Format("2006-01-02"), Langs: map[string]int{}, Head: r.Head, Partial: r.Partial}
	testCode := 0
	for _, f := range r.Files {
		s.Files++
		s.Code += f.Code
		s.Complexity += f.Complexity
		s.Todos += f.Todos
		s.Langs[f.Lang] += f.Code
		if f.Test {
			s.Tests++
			testCode += f.Code
		}
	}
	if s.Code > 0 {
		s.TestRatio = float64(testCode) / float64(s.Code)
	}
	return s
}
