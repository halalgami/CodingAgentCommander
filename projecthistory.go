package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// maxUnpinnedProjects is a safety valve, not a user-facing cap: history is
// effectively unbounded (distinct folders are few), but a pathological file is
// bounded by keeping all pinned entries + the most-recent this-many unpinned.
const maxUnpinnedProjects = 500

// ProjectEntry is one launched folder, persisted in projects.json. One row per
// folder; upserted on launch.
type ProjectEntry struct {
	Folder      string `json:"folder"`          // key: filepath.Clean(abs path)
	Label       string `json:"label,omitempty"` // default = basename; user-renamable
	LastModelID string `json:"lastModelID"`
	LastOpened  int64  `json:"lastOpened"` // unix milliseconds
	OpenCount   int    `json:"openCount"`
	Pinned      bool   `json:"pinned"`
}

// ProjectView is ProjectEntry plus a transient, NON-persisted existence flag for
// the UI (does the folder still exist on disk?). Returned by ListProjects only.
type ProjectView struct {
	Folder      string `json:"folder"`
	Label       string `json:"label"`
	LastModelID string `json:"lastModelID"`
	LastOpened  int64  `json:"lastOpened"`
	OpenCount   int    `json:"openCount"`
	Pinned      bool   `json:"pinned"`
	Missing     bool   `json:"missing"`
}

type projectHistory struct {
	Projects []ProjectEntry `json:"projects"`
}

// projectHistoryPath sits beside config.toml; configPath() honors
// COMMANDER_CONFIG, so redirecting that in tests redirects this too.
func projectHistoryPath() string {
	return filepath.Join(filepath.Dir(configPath()), "projects.json")
}

// cleanFolderKey normalizes a folder path so /foo and /foo/ (and relatives)
// don't create duplicate rows. Empty in -> empty out (caller skips).
func cleanFolderKey(folder string) string {
	f := strings.TrimSpace(folder)
	if f == "" {
		return ""
	}
	if abs, err := filepath.Abs(f); err == nil {
		f = abs
	}
	return filepath.Clean(f)
}

func loadProjectHistory() projectHistory {
	b, err := os.ReadFile(projectHistoryPath())
	if err != nil {
		return projectHistory{}
	}
	var h projectHistory
	if json.Unmarshal(b, &h) != nil {
		return projectHistory{}
	}
	return h
}

// saveProjectHistory writes atomically (temp + rename) after pruning, so a crash
// mid-write can't leave a half-written file.
func saveProjectHistory(h projectHistory) error {
	p := projectHistoryPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(pruneProjects(h), "", "  ")
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// pruneProjects keeps every pinned entry and the most-recent maxUnpinnedProjects
// unpinned ones, dropping older unpinned beyond that. Order of the returned
// slice is not guaranteed (ListProjects sorts for display).
func pruneProjects(h projectHistory) projectHistory {
	var pinned, unpinned []ProjectEntry
	for _, e := range h.Projects {
		if e.Pinned {
			pinned = append(pinned, e)
		} else {
			unpinned = append(unpinned, e)
		}
	}
	if len(unpinned) > maxUnpinnedProjects {
		sort.SliceStable(unpinned, func(i, j int) bool { return unpinned[i].LastOpened > unpinned[j].LastOpened })
		unpinned = unpinned[:maxUnpinnedProjects]
	}
	return projectHistory{Projects: append(pinned, unpinned...)}
}

// sortProjects returns a copy sorted pinned-first, then most-recent first.
func sortProjects(in []ProjectEntry) []ProjectEntry {
	out := append([]ProjectEntry(nil), in...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Pinned != out[j].Pinned {
			return out[i].Pinned
		}
		return out[i].LastOpened > out[j].LastOpened
	})
	return out
}

// recordProjectOpen upserts the folder on launch. Never returns an error to the
// launch path — a save failure is logged but the launch still succeeds.
func (a *App) recordProjectOpen(folder, modelID string) {
	key := cleanFolderKey(folder)
	if key == "" {
		return
	}
	a.historyMu.Lock()
	defer a.historyMu.Unlock()
	h := loadProjectHistory()
	now := a.now().UnixMilli()
	for i := range h.Projects {
		if h.Projects[i].Folder == key {
			h.Projects[i].LastOpened = now
			h.Projects[i].OpenCount++
			h.Projects[i].LastModelID = modelID
			_ = saveProjectHistory(h)
			return
		}
	}
	h.Projects = append(h.Projects, ProjectEntry{
		Folder: key, Label: filepath.Base(key), LastModelID: modelID,
		LastOpened: now, OpenCount: 1,
	})
	_ = saveProjectHistory(h)
}

// ListProjects returns the display-sorted history with a fresh on-disk existence
// flag per entry. Bound to the frontend.
func (a *App) ListProjects() []ProjectView {
	a.historyMu.Lock()
	defer a.historyMu.Unlock()
	sorted := sortProjects(loadProjectHistory().Projects)
	out := make([]ProjectView, 0, len(sorted))
	for _, e := range sorted {
		label := e.Label
		if label == "" {
			label = filepath.Base(e.Folder)
		}
		missing := false
		if _, err := os.Stat(e.Folder); err != nil {
			missing = true
		}
		out = append(out, ProjectView{
			Folder: e.Folder, Label: label, LastModelID: e.LastModelID,
			LastOpened: e.LastOpened, OpenCount: e.OpenCount, Pinned: e.Pinned, Missing: missing,
		})
	}
	return out
}

func (a *App) PinProject(folder string, pinned bool) error {
	return a.mutateProject(folder, func(e *ProjectEntry) { e.Pinned = pinned })
}

func (a *App) RenameProject(folder, label string) error {
	return a.mutateProject(folder, func(e *ProjectEntry) {
		l := strings.TrimSpace(label)
		if l == "" {
			l = filepath.Base(e.Folder)
		}
		e.Label = l
	})
}

func (a *App) RemoveProject(folder string) error {
	key := cleanFolderKey(folder)
	a.historyMu.Lock()
	defer a.historyMu.Unlock()
	h := loadProjectHistory()
	kept := h.Projects[:0:0]
	for _, e := range h.Projects {
		if e.Folder != key {
			kept = append(kept, e)
		}
	}
	return saveProjectHistory(projectHistory{Projects: kept})
}

// ImportProjects seeds the store from folded legacy recents. It is a no-op if the
// store already has entries, so the one-time frontend migration is safe to
// re-run.
func (a *App) ImportProjects(entries []ProjectEntry) error {
	a.historyMu.Lock()
	defer a.historyMu.Unlock()
	if len(loadProjectHistory().Projects) > 0 {
		return nil
	}
	for i := range entries {
		entries[i].Folder = cleanFolderKey(entries[i].Folder)
		if entries[i].Label == "" {
			entries[i].Label = filepath.Base(entries[i].Folder)
		}
	}
	return saveProjectHistory(projectHistory{Projects: entries})
}

// mutateProject loads, finds by cleaned key, applies fn, and saves atomically.
func (a *App) mutateProject(folder string, fn func(*ProjectEntry)) error {
	key := cleanFolderKey(folder)
	a.historyMu.Lock()
	defer a.historyMu.Unlock()
	h := loadProjectHistory()
	for i := range h.Projects {
		if h.Projects[i].Folder == key {
			fn(&h.Projects[i])
			break
		}
	}
	return saveProjectHistory(h)
}
