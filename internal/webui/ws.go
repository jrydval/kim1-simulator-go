package webui

import (
	"context"
	"net/http"

	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

// stateMsg is broadcast to every connected client at a fixed UI rate.
type stateMsg struct {
	Type   string   `json:"type"` // "state"
	A      uint8    `json:"a"`
	X      uint8    `json:"x"`
	Y      uint8    `json:"y"`
	SP     uint8    `json:"sp"`
	PC     uint16   `json:"pc"`
	P      uint8    `json:"p"`
	Cycles uint64   `json:"cycles"`
	Digits [6]uint8 `json:"digits"`
	Halted bool     `json:"halted"`
	Error  string   `json:"error,omitempty"`
	SST    bool     `json:"sst"`
}

// clientMsg is sent by the browser: a keypad press/release, a control
// button (RS = hardware reset, ST = single-step/NMI — both wired directly
// on real KIM-1 hardware rather than scanned through the keypad matrix),
// or the SST slide switch's new position.
type clientMsg struct {
	Type string `json:"type"` // "key" | "reset" | "nmi" | "sst"
	Row  int    `json:"row"`
	Col  int    `json:"col"`
	Down bool   `json:"down"`
}

type client struct {
	conn *websocket.Conn
	send chan stateMsg
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// This is a local development tool (VS Code Simple Browser or a
		// plain localhost browser tab); there's no cross-origin risk
		// worth enforcing strict origin checks over.
		InsecureSkipVerify: true,
	})
	if err != nil {
		return
	}
	defer conn.CloseNow()

	cl := &client{conn: conn, send: make(chan stateMsg, 4)}
	s.registerClient(cl)
	defer s.unregisterClient(cl)

	ctx := r.Context()
	go s.writeLoop(ctx, cl)

	for {
		var msg clientMsg
		if err := wsjson.Read(ctx, conn, &msg); err != nil {
			return
		}
		s.handleClientMsg(msg)
	}
}

func (s *Server) writeLoop(ctx context.Context, cl *client) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-cl.send:
			if !ok {
				return
			}
			if err := wsjson.Write(ctx, cl.conn, msg); err != nil {
				return
			}
		}
	}
}

func (s *Server) handleClientMsg(msg clientMsg) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch msg.Type {
	case "key":
		s.sys.Keypad.SetPressed(msg.Row, msg.Col, msg.Down)
	case "reset":
		s.sys.Reset()
		s.halted = false
		s.haltReason = ""
	case "nmi":
		s.sys.CPU.NMI()
	case "sst":
		s.sys.SST = msg.Down
	}
}

func (s *Server) registerClient(c *client) {
	s.clientsMu.Lock()
	s.clients[c] = struct{}{}
	s.clientsMu.Unlock()
}

func (s *Server) unregisterClient(c *client) {
	s.clientsMu.Lock()
	delete(s.clients, c)
	s.clientsMu.Unlock()
	close(c.send)
}
