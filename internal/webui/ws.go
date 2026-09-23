package webui

import (
	"context"
	"net/http"

	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

// stateMsg is broadcast to every connected client at a fixed UI rate.
type stateMsg struct {
	Type       string    `json:"type"` // "state"
	A          uint8     `json:"a"`
	X          uint8     `json:"x"`
	Y          uint8     `json:"y"`
	SP         uint8     `json:"sp"`
	PC         uint16    `json:"pc"`
	P          uint8     `json:"p"`
	Cycles     uint64    `json:"cycles"`
	Digits     [6]uint8  `json:"digits"`
	Halted     bool      `json:"halted"`
	Error      string    `json:"error,omitempty"`
	SST        bool      `json:"sst"`
	Instr      string    `json:"instr"`
	AppPA      portState `json:"appPA"`
	AppPB      portState `json:"appPB"`
	KbdPA      portState `json:"kbdPA"`
	KbdPB      portState `json:"kbdPB"`
	TTYSelect  bool      `json:"ttySelect"`
	TTYLog     []ttyRun  `json:"ttyLog"`
	AppSwitchA uint8     `json:"appSwitchA"`
	AppSwitchB uint8     `json:"appSwitchB"`
	MemAddr    uint16    `json:"memAddr"`
	Mem        []int     `json:"mem"`
	Trail      []trailPC `json:"trail"`
}

// trailPC is one recently executed instruction: its address and byte
// length, so the memory viewer can light the whole instruction.
type trailPC struct {
	Addr uint16 `json:"addr"`
	Len  int    `json:"len"`
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
// position, a line of text typed into the TTY terminal panel, an App
// RIOT input-switch position, or a PAP/Intel HEX file to load straight
// into memory (bypassing the TTY entirely -- see handleClientMsg's
// "load" case).
type clientMsg struct {
	Type   string `json:"type"` // "key" | "reset" | "nmi" | "sst" | "ttyselect" | "ttysend" | "appswitch" | "memview" | "memwrite" | "load"
	Row    int    `json:"row"`
	Col    int    `json:"col"`
	Down   bool   `json:"down"`
	Text   string `json:"text"`
	Port   string `json:"port"`   // "appswitch": "A" or "B"
	Bit    int    `json:"bit"`    // "appswitch": 0-7
	Addr   int    `json:"addr"`   // "memview" / "memwrite"
	Val    int    `json:"val"`    // "memwrite"
	Format string `json:"format"` // "load": "pap" or "hex"
}

// loadResultMsg is sent back to the requesting client only (not
// broadcast) in response to a "load" message, reporting how it went.
type loadResultMsg struct {
	Type    string `json:"type"` // "loadresult"
	OK      bool   `json:"ok"`
	Message string `json:"message"`
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
		if result := s.handleClientMsg(msg); result != nil {
			if err := wsjson.Write(ctx, conn, result); err != nil {
				return
			}
		}
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

// handleClientMsg applies msg to the system and, for message types that
// need a direct reply rather than waiting for the next broadcast (only
// "load" today), returns it; nil otherwise.
func (s *Server) handleClientMsg(msg clientMsg) *loadResultMsg {
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
		s.appendTTY(msg.Text, true)
	case "memview":
		s.memViewAddr = uint16(msg.Addr) &^ (memViewBytes - 1)
	case "memwrite":
		addr := uint16(msg.Addr)
		if _, ok := s.sys.Peek(addr); ok {
			s.sys.Write(addr, uint8(msg.Val))
		}
	case "appswitch":
		if len(msg.Port) == 1 {
			s.sys.SetAppSwitch(msg.Port[0], msg.Bit, msg.Down)
		}
	case "load":
		return s.handleLoad(msg.Format, msg.Text)
	}
	return nil
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
