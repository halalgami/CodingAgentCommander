package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/halalgami/CodingAgentCommander/internal/repoid"
	"github.com/halalgami/CodingAgentCommander/internal/rooms"
	"github.com/halalgami/CodingAgentCommander/internal/rooms/netsel"
)

// Commander rooms (spec docs/superpowers/specs/2026-11-15-commander-rooms-design.md).
// Off until the user sets rooms up in the drawer; no Anthropic calls anywhere.

type roomsState struct {
	svc *rooms.Service
}

func (a *App) roomsDir() string { return filepath.Join(filepath.Dir(configPath()), "rooms") }

func (a *App) openRooms() error {
	s := &rooms.Service{Dir: a.roomsDir(), ListenIPs: a.roomsListenIPs, OnChange: func() { a.emitter.Emit("rooms:changed") }}
	// The 4-word check is usually read on the other machine: say so here too.
	s.OnConfirm = func(p rooms.Pending) {
		if a.notifier != nil {
			_ = a.notifier.Notify("Commander", p.Name+" wants to join — check the 4 words")
		}
	}
	if err := s.Open(); err != nil {
		return err
	}
	a.roomsSvc.svc = s
	return nil
}

func (a *App) closeRooms() {
	if a.roomsSvc.svc != nil {
		a.roomsSvc.svc.Close()
	}
}

func (a *App) rs() (*rooms.Service, error) {
	if a.roomsSvc.svc == nil {
		return nil, errors.New("rooms are not available")
	}
	return a.roomsSvc.svc, nil
}

func deviceName() string {
	h, _ := os.Hostname()
	return strings.TrimSuffix(h, ".local")
}

func (a *App) RoomsStatus() rooms.Status {
	s, err := a.rs()
	if err != nil {
		return rooms.Status{}
	}
	st := s.Status()
	st.Machine = deviceName()
	if !st.SetUp && st.Name == "" {
		st.Name = osFullName() // prefill only (UX spec §5)
	}
	return st
}

// RoomsSetup: first=true makes this the first machine (name = your display
// name); first=false prepares to pair with your first machine.
func (a *App) RoomsSetup(first bool, name string) error {
	s, err := a.rs()
	if err != nil {
		return err
	}
	if first && strings.TrimSpace(name) == "" {
		return errors.New("type your name")
	}
	return s.Setup(first, name, deviceName())
}

func netOf(n string) (netsel.Network, error) {
	if !netsel.Valid(n) {
		return "", errors.New("choose Local network or Tailscale")
	}
	return netsel.Network(n), nil
}

func (a *App) PairCode(network string) (rooms.CodeView, error) {
	s, err := a.rs()
	if err != nil {
		return rooms.CodeView{}, err
	}
	n, err := netOf(network)
	if err != nil {
		return rooms.CodeView{}, err
	}
	return s.PairCode(n)
}

func (a *App) InviteCode(room, network string) (rooms.CodeView, error) {
	s, err := a.rs()
	if err != nil {
		return rooms.CodeView{}, err
	}
	n, err := netOf(network)
	if err != nil {
		return rooms.CodeView{}, err
	}
	return s.InviteCode(room, n)
}

// JoinCode enters a pairing or invite code; folder binds an invited room.
func (a *App) JoinCode(code, folder string) (rooms.CodeView, error) {
	s, err := a.rs()
	if err != nil {
		return rooms.CodeView{}, err
	}
	return s.Join(code, deviceName(), folder)
}

func (a *App) ConfirmCode(ok bool) error {
	s, err := a.rs()
	if err != nil {
		return err
	}
	return s.Confirm(ok)
}

func (a *App) CancelCode() {
	if s, err := a.rs(); err == nil {
		s.CancelCode()
	}
}

// CreateRoom makes a room for a project folder; the project identity is the
// folder's normalised git remote when there is one.
func (a *App) CreateRoom(folder, name string) (rooms.RoomSummary, error) {
	s, err := a.rs()
	if err != nil {
		return rooms.RoomSummary{}, err
	}
	if strings.TrimSpace(name) == "" {
		name = filepath.Base(folder)
	}
	return s.CreateRoom(folder, name, projectIdentity(folder))
}

func (a *App) BindRoomFolder(room, folder string) error {
	s, err := a.rs()
	if err != nil {
		return err
	}
	return s.BindFolder(room, folder)
}

func (a *App) Rooms() []rooms.RoomSummary {
	s, err := a.rs()
	if err != nil {
		return []rooms.RoomSummary{}
	}
	return s.Rooms()
}

func (a *App) RoomView(room string) (rooms.RoomView, error) {
	s, err := a.rs()
	if err != nil {
		return rooms.RoomView{}, err
	}
	return s.View(room)
}

func (a *App) PostRoomMessage(room, kind, text, parent string, fields map[string]string) error {
	s, err := a.rs()
	if err != nil {
		return err
	}
	return s.Post(room, kind, text, parent, fields)
}

func (a *App) MarkRoomSeen(room, ref string) error {
	s, err := a.rs()
	if err != nil {
		return err
	}
	return s.MarkSeen(room, ref)
}

func (a *App) SetDeviceNetwork(device, network string) error {
	if !netsel.Valid(network) {
		return errors.New("choose Local network or Tailscale")
	}
	s, err := a.rs()
	if err != nil {
		return err
	}
	return s.SetDeviceNetwork(device, network)
}

// PickRoomFolder opens a folder picker for binding a room.
func (a *App) PickRoomFolder() (string, error) {
	if a.ctx == nil {
		return "", errors.New("no window")
	}
	return wruntime.OpenDirectoryDialog(a.ctx, wruntime.OpenDialogOptions{Title: "Choose the project folder for this room"})
}

// QueuedCount: my room messages no other machine holds yet.
func (a *App) QueuedCount() int {
	if a.roomsQueued != nil {
		return a.roomsQueued()
	}
	s, err := a.rs()
	if err != nil {
		return 0
	}
	return s.QueuedCount()
}

// beforeClose is the Wails OnBeforeClose hook: while some of my messages
// have not left this machine, ask first (spec §9 quit guard).
func (a *App) beforeClose(_ context.Context) bool {
	if a.quitAnyway.Load() {
		return false
	}
	if n := a.QueuedCount(); n > 0 {
		a.emitter.Emit("rooms:quitguard", n)
		return true
	}
	return false
}

// QuitAnyway quits even though messages are still queued.
func (a *App) QuitAnyway() {
	a.quitAnyway.Store(true)
	if a.ctx != nil {
		wruntime.Quit(a.ctx)
	}
}

var _ = atomic.Bool{}

// projectIdentity: the folder's repository identity (its root commit — the
// same identity the ledger uses, stable across clones and paths); "" outside git.
func projectIdentity(folder string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	id := repoid.NewResolver().Resolve(ctx, folder)
	if strings.HasPrefix(id.Identity, "rootcommit:") {
		return id.Identity
	}
	return ""
}
