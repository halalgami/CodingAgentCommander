package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/halalgami/CodingAgentCommander/internal/rooms/netsel"
)

func roomsTestApp(t *testing.T) *App {
	t.Helper()
	keyring.MockInit()
	netsel.AllowLoopback = true
	t.Setenv("COMMANDER_CONFIG", filepath.Join(t.TempDir(), "config.toml"))
	a := NewApp()
	a.emitter = &fakeEmitter{}
	a.roomsListenIPs = []string{"127.0.0.1"}
	if err := a.openRooms(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.closeRooms)
	return a
}

func TestRoomsOffUntilSetUp(t *testing.T) {
	a := roomsTestApp(t)
	st := a.RoomsStatus()
	if st.SetUp || st.Joining || len(a.Rooms()) != 0 {
		t.Fatalf("status %+v", st)
	}
	if _, err := os.Stat(a.roomsDir()); err == nil {
		if ents, _ := os.ReadDir(a.roomsDir()); len(ents) != 0 {
			t.Fatal("rooms wrote files before setup")
		}
	}
	if a.QueuedCount() != 0 {
		t.Fatal("queued without rooms")
	}
}

func TestRoomsSetupCreateAndRestart(t *testing.T) {
	a := roomsTestApp(t)
	if err := a.RoomsSetup(true, "Hussain"); err != nil {
		t.Fatal(err)
	}
	r, err := a.CreateRoom(t.TempDir(), "Ayina")
	if err != nil || r.Name != "Ayina" {
		t.Fatalf("create %+v %v", r, err)
	}
	if err := a.PostRoomMessage(r.ID, "note", "hi", "", nil); err != nil {
		t.Fatal(err)
	}
	if a.QueuedCount() != 0 { // no other device in the room: nothing is waiting
		t.Fatalf("queued %d", a.QueuedCount())
	}
	if err := a.SetDeviceNetwork("x", "internet"); err == nil {
		t.Fatal("bad network accepted")
	}
	a.closeRooms()
	b := NewApp()
	b.emitter = &fakeEmitter{}
	b.roomsListenIPs = []string{"127.0.0.1"}
	b.openRooms()
	t.Cleanup(b.closeRooms)
	if st := b.RoomsStatus(); !st.SetUp || st.Name != "Hussain" || len(b.Rooms()) != 1 {
		t.Fatalf("after restart %+v", st)
	}
}

func TestQuitGuardOnlyWhenQueued(t *testing.T) {
	a := roomsTestApp(t)
	if a.beforeClose(nil) {
		t.Fatal("blocked quit with rooms off")
	}
	a.roomsQueued = func() int { return 2 }
	if !a.beforeClose(nil) {
		t.Fatal("did not guard queued messages")
	}
	if n := countEvents(a.emitter.(*fakeEmitter), "rooms:quitguard"); n != 1 {
		t.Fatalf("quitguard events %d", n)
	}
	a.quitAnyway.Store(true)
	if a.beforeClose(nil) {
		t.Fatal("Quit anyway still blocked")
	}
}
