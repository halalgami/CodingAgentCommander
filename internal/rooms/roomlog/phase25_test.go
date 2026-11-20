package roomlog

import (
	"testing"

	"github.com/halalgami/CodingAgentCommander/internal/rooms/ident"
)

func TestLiveSessionsFromStatus(t *testing.T) {
	a, b := closeRoom(t)
	b.post(t, KindStatus, Status{Event: EvStarted, Session: "s1", Label: "api", Branch: "main"})
	b.post(t, KindStatus, Status{Event: EvTitle, Session: "s1", Title: "Fix login"})
	b.post(t, KindStatus, Status{Event: EvStarted, Session: "s2", Label: "web"})
	b.post(t, KindStatus, Status{Event: EvEnded, Session: "s2", Reason: "killed"})
	syncTo(t, b, a)
	st := Replay(a.lane.All())
	if len(st.Live) != 1 {
		t.Fatalf("live = %+v", st.Live)
	}
	got := st.Live[b.id.PeerID()+"|s1"]
	if got.Label != "api" || got.Title != "Fix login" || got.Branch != "main" {
		t.Fatalf("merged live = %+v", got)
	}
}

func TestStatusAfterCloseDropped(t *testing.T) {
	a, b := closeRoom(t)
	a.post(t, KindRoomClose, Close{Heads: a.lane.Heads()})
	syncTo(t, a, b)
	b.post(t, KindStatus, Status{Event: EvStarted, Session: "late"})
	syncTo(t, b, a)
	if st := Replay(a.lane.All()); len(st.Live) != 0 {
		t.Fatalf("status after close accepted: %+v", st.Live)
	}
}

func TestClaimsConflictAndRelease(t *testing.T) {
	a, b := closeRoom(t)
	root := a.post(t, KindMsg, Msg{Kind: "handoff", Text: "do it"})
	syncTo(t, a, b)
	a.post(t, KindClaim, Claim{Thread: root.Ref(), Session: "sa"})
	b.post(t, KindClaim, Claim{Thread: root.Ref(), Session: "sb"})
	syncTo(t, b, a)
	syncTo(t, a, b)
	st := Replay(a.lane.All())
	if n := len(st.Claims[root.Ref()]); n != 2 {
		t.Fatalf("claims = %+v", st.Claims)
	}
	b.post(t, KindClaimRelease, Claim{Thread: root.Ref()})
	syncTo(t, b, a)
	st = Replay(a.lane.All())
	if c := st.Claims[root.Ref()]; len(c) != 1 || c[0].Device != a.id.PeerID() || c[0].Session != "sa" {
		t.Fatalf("after release = %+v", c)
	}
}

func TestClaimOnUnknownThreadIgnored(t *testing.T) {
	a, _ := closeRoom(t)
	a.post(t, KindClaim, Claim{Thread: "nope:1"})
	if st := Replay(a.lane.All()); len(st.Claims) != 0 {
		t.Fatalf("claims = %+v", st.Claims)
	}
}

func TestPickedUpDelivery(t *testing.T) {
	a, b := closeRoom(t)
	m := a.post(t, KindMsg, Msg{Kind: "handoff", Text: "x"})
	syncTo(t, a, b)
	b.post(t, KindReceipt, Receipt{Lane: m.Device, Seq: m.Seq, State: "picked-up"})
	syncTo(t, b, a)
	st := Replay(a.lane.All())
	d := DeliveryOf(m, []string{a.id.PeerID(), b.id.PeerID()}, IndexReceipts(st.Accepted), nil)
	if d[b.id.PeerID()] != PickedUp {
		t.Fatalf("delivery = %+v", d)
	}
	if !IndexReceipts(st.Accepted).PickedUpBy(m.Ref(), b.id.PeerID()) {
		t.Fatal("PickedUpBy false")
	}
}

func TestPreRevokeOnlyBindsTheSignersOwnDevice(t *testing.T) {
	a, b := closeRoom(t)
	x := newUser(t, "x") // a device of nobody in this room yet
	// b (a member, not x's user) signs a revoke for x's device ahead of time.
	uk, _ := b.st.UserKey()
	b.post(t, KindDeviceRevoke, SignRevoke(uk, x.id.Device, 0))
	syncTo(t, b, a)
	st := Replay(a.lane.All())
	if st.Revoked[x.id.PeerID()] {
		t.Fatal("a colleague's pre-revoke marked another user's device revoked")
	}
	if !st.PreRevoked[x.id.PeerID()][string(b.id.User)] {
		t.Fatalf("pre-revoke not recorded under its signer: %v", st.PreRevoked)
	}
	// A squatter's earlier pre-revoke must not hide a later signer's one.
	auk, _ := a.st.UserKey()
	a.post(t, KindDeviceRevoke, SignRevoke(auk, x.id.Device, 0))
	st = Replay(a.lane.All())
	if !st.PreRevoked[x.id.PeerID()][string(a.id.User)] || !st.PreRevoked[x.id.PeerID()][string(b.id.User)] {
		t.Fatalf("second signer lost: %v", st.PreRevoked)
	}
}

func TestPreRevokeStaysPendingUntilCertified(t *testing.T) {
	a, b := closeRoom(t)
	uk, _ := b.st.UserKey()
	b2, err := b.st.SignCert(newUser(t, "tmp").id.Device, "b2") // a second device of b's user
	if err != nil {
		t.Skip("no second-device cert helper:", err)
	}
	b.post(t, KindDeviceRevoke, SignRevoke(uk, b2.Device, 0))
	syncTo(t, b, a)
	st := Replay(a.lane.All())
	if st.Revoked[ident.PeerIDOf(b2.Device)] {
		t.Fatal("revoked before the device arrived")
	}
	if !st.PreRevoked[ident.PeerIDOf(b2.Device)][string(b.id.User)] {
		t.Fatal("not pending")
	}
}
