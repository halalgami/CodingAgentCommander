package p2p

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"

	"github.com/halalgami/CodingAgentCommander/internal/rooms/netsel"
)

// One request, one response, each a length-prefixed frame with a size cap.
// Handlers learn the caller and the network it came in on (D10).

const rpcTimeout = 3 * time.Minute // pairing waits for a human confirm

func writeFrame(w io.Writer, b []byte) error {
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(b)))
	if _, err := w.Write(n[:]); err != nil {
		return err
	}
	_, err := w.Write(b)
	return err
}

func readFrame(r io.Reader, max int) ([]byte, error) {
	var n [4]byte
	if _, err := io.ReadFull(r, n[:]); err != nil {
		return nil, err
	}
	size := int(binary.BigEndian.Uint32(n[:]))
	if size > max {
		return nil, fmt.Errorf("frame of %d bytes over the %d cap", size, max)
	}
	b := make([]byte, size)
	_, err := io.ReadFull(r, b)
	return b, err
}

type Handler func(from peer.ID, net netsel.Network, req []byte) ([]byte, error)

// Handle serves proto. limit gives each caller its frame cap before anything
// is read (0 = refuse), so strangers can't make us allocate. A handler error
// closes the stream without a reply.
func (n *Node) Handle(proto string, limit func(peer.ID) int, h Handler) {
	n.h.SetStreamHandler(protocol.ID(proto), func(s network.Stream) {
		defer s.Close()
		s.SetDeadline(time.Now().Add(rpcTimeout))
		net, ok := netsel.OfAddr(s.Conn().RemoteMultiaddr())
		if !ok {
			s.Reset()
			return
		}
		maxReq := limit(s.Conn().RemotePeer())
		if maxReq <= 0 {
			s.Reset()
			return
		}
		req, err := readFrame(s, maxReq)
		if err != nil {
			s.Reset()
			return
		}
		resp, err := h(s.Conn().RemotePeer(), net, req)
		if err != nil {
			s.Reset()
			return
		}
		writeFrame(s, resp)
	})
}

// Request sends one request and waits for the reply (at most maxResp bytes).
func (n *Node) Request(ctx context.Context, id peer.ID, proto string, req []byte, maxResp int) ([]byte, error) {
	s, err := n.h.NewStream(ctx, id, protocol.ID(proto))
	if err != nil {
		return nil, err
	}
	defer s.Close()
	if dl, ok := ctx.Deadline(); ok {
		s.SetDeadline(dl)
	} else {
		s.SetDeadline(time.Now().Add(rpcTimeout))
	}
	if err := writeFrame(s, req); err != nil {
		return nil, err
	}
	s.CloseWrite()
	resp, err := readFrame(s, maxResp)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("the other machine refused the request")
		}
		return nil, err
	}
	return resp, nil
}
