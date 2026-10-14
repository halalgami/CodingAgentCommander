package main

import (
	"fmt"
	"os"
	"time"

	"github.com/halalgami/CodingAgentCommander/internal/city"
	"github.com/halalgami/CodingAgentCommander/internal/codecity"
	"github.com/halalgami/CodingAgentCommander/internal/ledger/ingest"
	"github.com/halalgami/CodingAgentCommander/internal/ledger/live"
	"github.com/halalgami/CodingAgentCommander/internal/ledger/store"
	"github.com/halalgami/CodingAgentCommander/internal/metrics/scan"
	"github.com/halalgami/CodingAgentCommander/internal/progress"
)

type LedgerStatus struct {
	Enabled     bool            `json:"enabled"`
	StoreTitles bool            `json:"storeTitles"`
	ReadOnly    bool            `json:"readOnly"`
	Excluded    []string        `json:"excluded"`
	Progress    ingest.Progress `json:"progress"`
	LastError   string          `json:"lastError"`
	Dir         string          `json:"dir"`
	// HasData is true while a ledger directory exists, even when recording is
	// off, so the UI can always offer to delete it.
	HasData bool `json:"hasData"`
}

type DashSnapshot struct {
	Sessions []live.Session                  `json:"sessions"`
	Projects []store.ProjectInfo             `json:"projects"`
	Timeline map[string][]live.TimelineEvent `json:"timeline"`
}

func (a *App) LedgerStatus() LedgerStatus {
	l := a.ledger
	s := LedgerStatus{Dir: a.ledgerDir(), Excluded: []string{}}
	if _, err := os.Stat(a.ledgerDir()); err == nil {
		s.HasData = true
	}
	var st *store.Store
	l.mu.Lock()
	st, s.Progress = l.store, l.progress
	l.mu.Unlock()
	if st != nil {
		c := st.Consent()
		s.Enabled, s.StoreTitles, s.ReadOnly, s.LastError = true, c.StoreTitles, st.ReadOnly(), st.LastError()
		if c.Excluded != nil {
			s.Excluded = c.Excluded
		}
	}
	return s
}

// SetLedgerConsent is the opt-in/out switch. Turning it off stops ingestion and
// closes the store but keeps the data; DeleteLedger removes it.
func (a *App) SetLedgerConsent(on, storeTitles bool) (LedgerStatus, error) {
	l := a.ledger
	l.consentMu.Lock()
	defer l.consentMu.Unlock()
	if !on {
		l.stop()
		defer a.cityChanged()
		return a.LedgerStatus(), store.Disable(a.ledgerDir())
	}
	if st := l.currentStore(); st != nil {
		c := st.Consent()
		c.StoreTitles = storeTitles
		return a.LedgerStatus(), st.SetConsent(c)
	}
	st, err := store.Create(a.ledgerDir(), store.Consent{StoreTitles: storeTitles}, time.Now)
	if err != nil {
		return a.LedgerStatus(), err
	}
	l.attach(st)
	return a.LedgerStatus(), nil
}

func (a *App) SetLedgerExcluded(paths []string) error {
	st := a.ledger.currentStore()
	if st == nil {
		return store.ErrNoConsent
	}
	c := st.Consent()
	c.Excluded = paths
	defer a.cityChanged()
	return st.SetConsent(c)
}

func (a *App) ForgetProject(key string) error {
	st := a.ledger.currentStore()
	if st == nil {
		return store.ErrNoConsent
	}
	defer a.cityChanged()
	return st.Forget(key)
}

func (a *App) DeleteLedger() error {
	a.ledger.consentMu.Lock()
	defer a.ledger.consentMu.Unlock()
	a.ledger.stop()
	defer a.cityChanged()
	return store.DeleteAll(a.ledgerDir())
}

// cityChanged tells the dashboard to refetch CityView after a history change
// the tail pump does not see (forget, exclude, opt-out, delete).
func (a *App) cityChanged() { a.emitter.Emit("dash:city") }

func (a *App) DashSnapshot() DashSnapshot {
	d := DashSnapshot{Sessions: a.ledger.tracker.Snapshot(), Projects: []store.ProjectInfo{}, Timeline: a.ledger.tracker.Timeline()}
	if st := a.ledger.currentStore(); st != nil {
		d.Projects = st.Projects()
	}
	roots := a.ledger.roots()
	for i := range d.Sessions {
		d.Sessions[i] = relTargets(d.Sessions[i], roots)
	}
	return d
}

// SetLiveFocus switches the tail cadence: 1 s while the window is focused, else 5 s.
func (a *App) SetLiveFocus(focused bool) { a.ledger.focused.Store(focused) }

type ProjectMetricsView struct {
	Files     map[string]scan.FileMetrics `json:"files"`
	Snapshots []scan.Snapshot             `json:"snapshots"`
}

func (a *App) ProjectMetrics(key string) ProjectMetricsView {
	v := ProjectMetricsView{Files: map[string]scan.FileMetrics{}, Snapshots: []scan.Snapshot{}}
	if st := a.ledger.currentStore(); st != nil {
		if f, s, ok := st.Metrics(key); ok {
			v.Files, v.Snapshots = f, s
		}
	}
	return v
}

type CommanderView struct {
	Level   int            `json:"level"`
	XP      float64        `json:"xp"`
	Next    float64        `json:"next"`
	Unlocks []store.Unlock `json:"unlocks"`
}

type ProjectProgress struct {
	Key        string          `json:"key"`
	Label      string          `json:"label"`
	Rank       string          `json:"rank"`
	Hours      float64         `json:"hours"`
	Difficulty float64         `json:"difficulty"`
	Heroes     []progress.Hero `json:"heroes"`
	Unlocks    []store.Unlock  `json:"unlocks"`
}

type ProgressView struct {
	Commander CommanderView     `json:"commander"`
	Projects  []ProjectProgress `json:"projects"`
	Live      []progress.Hero   `json:"live"`
	Locked    bool              `json:"locked"`
}

func (a *App) Progress() ProgressView { return a.ledger.progressView() }

func (a *App) SessionHero(id string) progress.Hero {
	for _, p := range a.ledger.progressView().Projects {
		for _, h := range p.Heroes {
			if h.SessionID == id {
				return h
			}
		}
	}
	return progress.Hero{SessionID: id, Level: 1, Title: progress.Title(1)}
}

type PowerInfo struct {
	LowPower bool `json:"lowPower"`
}

var lowPowerFn = lowPowerMode

// PowerState lets the dashboard go still in Low Power Mode.
func (a *App) PowerState() PowerInfo { return PowerInfo{LowPower: lowPowerFn()} }

// CityView is the Al Modon city (phase 5a). History off → locked, no districts.
func (a *App) CityView() city.CityView {
	st := a.ledger.currentStore()
	if st == nil {
		return city.CityView{Districts: []city.District{}, Locked: true}
	}
	var ps []city.ProjectData
	for _, p := range st.Projects() {
		ps = append(ps, city.ProjectData{Info: p, Sessions: st.Sessions(p.Key), Unlocks: st.Unlocks(p.Key)})
	}
	return city.Build(ps, a.ledger.providers(), time.Now())
}

// CodeView is one project's CodeCity (phase 5b). History off → locked.
func (a *App) CodeView(key string) codecity.CodeView {
	locked := codecity.CodeView{Key: key, Locked: true, Folders: []codecity.Folder{}, Files: []codecity.File{}, Slabs: []codecity.Slab{}}
	st := a.ledger.currentStore()
	if st == nil {
		return locked
	}
	var info store.ProjectInfo
	for _, p := range st.Projects() {
		if p.Key == key {
			info = p
		}
	}
	if info.Key == "" {
		return locked
	}
	metrics, _, _ := st.Metrics(key)
	targets := liveTargets(a.ledger.tracker.Snapshot(), st.Roots(), info.Root)
	return codecity.Build(info, metrics, st.Touches(key), targets, time.Now())
}

// ScanProject asks the metrics scheduler to scan this project now.
func (a *App) ScanProject(key string) error {
	st := a.ledger.currentStore()
	if st == nil {
		return store.ErrNoConsent
	}
	root := st.Roots()[key]
	if root == "" {
		return fmt.Errorf("unknown project")
	}
	if _, err := os.Stat(root); err != nil {
		return fmt.Errorf("project folder is missing")
	}
	a.ledger.sched.HeadMoved(root)
	return nil
}
