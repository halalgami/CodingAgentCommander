// Package repoid decides which project a working directory belongs to.
// Identity is the repo's root commit when there is one (it survives moves,
// renames and re-clones), else the cleaned path.
package repoid

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/halalgami/CodingAgentCommander/internal/safegit"
)

type ID struct {
	Key      string `json:"key"`
	Identity string `json:"identity"`
	Root     string `json:"root"`
	Label    string `json:"label"`
	PathKey  string `json:"pathKey"` // key this root would have as a path: project (for migration)
	Gone     string `json:"-"`       // the vanished cwd this ID was walked up from, if any
}

func KeyOf(identity string) string {
	h := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(h[:8])
}

type Resolver struct {
	mu    sync.Mutex
	cache map[string]ID
	tops  map[string]string
}

func NewResolver() *Resolver { return &Resolver{cache: map[string]ID{}, tops: map[string]string{}} }

func (r *Resolver) Resolve(ctx context.Context, cwd string) ID {
	c := clean(cwd)
	r.mu.Lock()
	id, ok := r.cache[c]
	r.mu.Unlock()
	if ok {
		return id
	}
	id = resolve(ctx, c)
	r.mu.Lock()
	r.cache[c] = id
	r.mu.Unlock()
	return id
}

func pathID(root string) ID {
	ident := "path:" + root
	k := KeyOf(ident)
	return ID{Key: k, Identity: ident, Root: root, Label: filepath.Base(root), PathKey: k}
}

func resolve(ctx context.Context, c string) ID {
	if _, err := os.Stat(c); errors.Is(err, fs.ErrNotExist) {
		return resolveGone(ctx, c)
	}
	top, err := safegit.Run(ctx, c, "rev-parse", "--show-toplevel")
	if err != nil || top == "" {
		return pathID(c)
	}
	root := clean(top)
	if common, err := safegit.Run(ctx, c, "rev-parse", "--path-format=absolute", "--git-common-dir"); err == nil && filepath.Base(common) == ".git" {
		root = clean(filepath.Dir(common)) // a worktree belongs to its main repo
	}
	id := pathID(root)
	roots, err := safegit.Run(ctx, root, "rev-list", "--max-parents=0", "HEAD")
	if err != nil || roots == "" {
		return id
	}
	shas := strings.Fields(roots)
	sort.Strings(shas)
	id.Identity = "rootcommit:" + shas[0]
	id.Key = KeyOf(id.Identity)
	return id
}

// resolveGone handles a cwd that no longer exists (a removed agent worktree,
// a deleted scratch folder). Inside a repo that still exists it belongs to
// that repo, and PathKey names the path-keyed project older ledgers recorded
// for it so the caller can fold it in. Anything else keeps its own path
// identity. A repo at $HOME (dotfiles) or / never adopts deleted folders.
func resolveGone(ctx context.Context, c string) ID {
	gone := pathID(c)
	anc := filepath.Dir(c)
	for {
		if _, err := os.Stat(anc); err == nil {
			break
		}
		next := filepath.Dir(anc)
		if next == anc {
			return gone
		}
		anc = next
	}
	id := resolve(ctx, anc)
	home, _ := os.UserHomeDir()
	if strings.HasPrefix(id.Identity, "path:") || id.Root == clean(home) || id.Root == string(filepath.Separator) {
		return gone
	}
	id.Gone, id.PathKey = c, gone.Key
	return id
}

// clean makes p absolute with symlinks resolved. For a path that no longer
// exists, the nearest existing ancestor is resolved and the rest re-joined,
// so a deleted folder keeps the key it had while it existed.
func clean(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	p = filepath.Clean(p)
	if real, err := filepath.EvalSymlinks(p); err == nil {
		return real
	}
	dir, rest := p, ""
	for parent := filepath.Dir(dir); parent != dir; parent = filepath.Dir(dir) {
		rest = filepath.Join(filepath.Base(dir), rest)
		dir = parent
		if real, err := filepath.EvalSymlinks(dir); err == nil {
			return filepath.Join(real, rest)
		}
	}
	return p
}

// Toplevel returns the working tree's own top-level directory for cwd (for a
// worktree, the worktree itself, not its main repo), symlinks resolved; "" if
// cwd is not inside a git working tree. Scans and hotspot lookups use this;
// persistence uses Resolve's main-repo identity.
func (r *Resolver) Toplevel(ctx context.Context, cwd string) string {
	c := clean(cwd)
	r.mu.Lock()
	t, ok := r.tops[c]
	r.mu.Unlock()
	if ok {
		return t
	}
	if top, err := safegit.Run(ctx, c, "rev-parse", "--show-toplevel"); err == nil && top != "" {
		t = clean(top)
	}
	r.mu.Lock()
	r.tops[c] = t
	r.mu.Unlock()
	return t
}
