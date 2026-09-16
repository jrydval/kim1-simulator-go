package webui

import (
	"context"
	"net/http"

	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

// stateMsg is broadcast to every connected client at a fixed UI rate.
type stateMsg struct {
	Type      string    `json:"type"` // "state"
	A         uint8     `json:"a"`
	X         uint8     `json:"x"`
	Y         uint8     `json:"y"`
	SP        uint8     `json:"sp"`
	PC        uint16    `json:"pc"`
	P         uint8     `json:"p"`
	Cycles    uint64    `json:"cycles"`
	Digits    [6]uint8  `json:"digits"`
	Halted    bool      `json:"halted"`
	Error     string    `json:"error,omitempty"`
	SST       bool      `json:"sst"`
	Instr     string    `json:"instr"`
	AppPA     portState `json:"appPA"`
	AppPB     portState `json:"appPB"`
	KbdPA     portState `json:"kbdPA"`
	KbdPB     portState `json:"kbdPB"`
	TTYSelect bool      `json:"ttySelect"`
	TTYOut    string    `json:"ttyOut"`
}

// portState is one RIOT I/O port's current electrical state: Value is the
// actual pin level (output bits as driven, input bits as last sampled),
// same as what a logic probe would read; DDR marks which bits are outputs
// (1) vs. inputs (0).
type portState struct {
	Value uint8 `json:"value"`
	DDR   uint8 `json:"ddr"`
}

// clientMsg is sent by the browser: a keypad press/release, a control
// button (RS = hardware reset, ST = single-step/NMI — both wired directly
// on real KIM-1 hardware rather than scanned through the keypad matrix),
// the SST slide switch's new position, the TTY/keyboard mode switch's new
// position, or a line of text typed into the TTY terminal panel.
type clientMsg struct {
	Type string `json:"type"` // "key" | "reset" | "nmi" | "sst" | "ttyselect" | "ttysend"
	Row  int    `json:"row"`
	Col  int    `json:"col"`
	Down bool   `json:"down"`
	Text string `json:"text"`
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
	case "ttyselect":
		s.sys.TTYSelect = msg.Down
	case "ttysend":
		s.sys.TTY.Send([]byte(msg.Text)...)
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
