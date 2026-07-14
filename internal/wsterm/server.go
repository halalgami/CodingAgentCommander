// Package wsterm streams a ptybridge over a websocket for xterm.js.
package wsterm

import (
	"crypto/subtle"
	"encoding/json"
	"net"
	"net/http"
	"net/url"

	"github.com/gorilla/websocket"

	"github.com/halalgami/CodingAgentCommander/internal/ptybridge"
)

// allowedOrigin permits connections only from the Wails webview (wails:// or the
// localhost asset host) or from non-browser clients (which send no Origin). A
// third-party web page trying to reach the loopback socket sends its own
// https://… Origin and is rejected — defense in depth behind the token.
func allowedOrigin(r *http.Request) bool {
	o := r.Header.Get("Origin")
	if o == "" {
		return true // native client / same-origin server request
	}
	u, err := url.Parse(o)
	if err != nil {
		return false
	}
	if u.Scheme == "wails" {
		return true
	}
	host := u.Hostname()
	if host == "localhost" || host == "wails.localhost" {
		return true
	}
	return net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback()
}

var upgrader = websocket.Upgrader{CheckOrigin: allowedOrigin}

// ControlMsg is a JSON text frame carrying a terminal control action.
type ControlMsg struct {
	Type     string `json:"type"` // "resize" | "select"
	Rows     uint16 `json:"rows"`
	Cols     uint16 `json:"cols"`
	WindowID string `json:"windowID"`
}

// Handler upgrades to a websocket, opens a bridge via open(), and pumps bytes
// both ways. Binary frames are terminal data; text frames are ControlMsg JSON.
//
// The /ws stream can read and inject keystrokes into the live Claude Code
// session, so it is gated by a per-run token (passed as ?token=): any local
// process or browser page that reaches the loopback port without it is refused
// the upgrade. An empty token disables the check (tests only).
func Handler(token string, open func() (*ptybridge.Bridge, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if token != "" && subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("token")), []byte(token)) != 1 {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		bridge, err := open()
		if err != nil {
			_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"error"}`))
			return
		}
		defer bridge.Close()

		// pty -> ws (binary)
		go func() {
			buf := make([]byte, 32*1024)
			for {
				n, err := bridge.Read(buf)
				if n > 0 {
					if werr := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); werr != nil {
						return
					}
				}
				if err != nil {
					_ = conn.Close()
					return
				}
			}
		}()

		// ws -> pty (binary = keystrokes; text = control)
		for {
			mt, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			switch mt {
			case websocket.BinaryMessage:
				_, _ = bridge.Write(data)
			case websocket.TextMessage:
				var m ControlMsg
				if json.Unmarshal(data, &m) != nil {
					continue
				}
				switch m.Type {
				case "resize":
					_ = bridge.Resize(m.Rows, m.Cols)
				case "select":
					_ = bridge.SelectWindow(m.WindowID)
				}
			}
		}
	}
}
