package webui

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"

	"6502/internal/kim1"
)

func TestLoadMessageOverWebSocket(t *testing.T) {
	sys := kim1.New()
	s := NewServer(sys)

	httpSrv := httptest.NewServer(s.Handler())
	defer httpSrv.Close()

	wsURL := "ws" + strings.TrimPrefix(httpSrv.URL, "http") + "/ws"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.CloseNow()

	// LL=01, addr=$0200, data=[$42]; checksum = 01+02+00+42 = 45.
	if err := wsjson.Write(ctx, conn, clientMsg{Type: "load", Format: "pap", Text: ";010200420045"}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	var result loadResultMsg
	if err := wsjson.Read(ctx, conn, &result); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if result.Type != "loadresult" || !result.OK {
		t.Fatalf("result = %+v, want a successful loadresult", result)
	}

	s.mu.Lock()
	v, _ := sys.Peek(0x0200)
	s.mu.Unlock()
	if v != 0x42 {
		t.Fatalf("Peek($0200) = %02X, want 42", v)
	}
}

func TestHandleLoadPAPWritesMemoryAndJumpsView(t *testing.T) {
	sys := kim1.New()
	s := NewServer(sys)

	// LL=02, addr=$0300, data=[$11,$22]; checksum = 02+03+00+11+22 = 38.
	result := s.handleClientMsg(clientMsg{Type: "load", Format: "pap", Text: ";0203001122" + "0038"})
	if result == nil || !result.OK {
		t.Fatalf("result = %+v, want OK", result)
	}
	if !strings.Contains(result.Message, "2 bytes") || !strings.Contains(result.Message, "0300") {
		t.Fatalf("result.Message = %q, want it to mention 2 bytes at $0300", result.Message)
	}
	if v, _ := sys.Peek(0x0300); v != 0x11 {
		t.Fatalf("Peek($0300) = %02X, want 11", v)
	}
	if v, _ := sys.Peek(0x0301); v != 0x22 {
		t.Fatalf("Peek($0301) = %02X, want 22", v)
	}
	if s.memViewAddr != 0x0300&^(memViewBytes-1) {
		t.Fatalf("memViewAddr = %04X, want the page containing $0300", s.memViewAddr)
	}
}

func TestHandleLoadHexWritesMemory(t *testing.T) {
	sys := kim1.New()
	s := NewServer(sys)

	result := s.handleClientMsg(clientMsg{Type: "load", Format: "hex", Text: ":02010000214696\n:00000001FF\n"})
	if result == nil || !result.OK {
		t.Fatalf("result = %+v, want OK", result)
	}
	if v, _ := sys.Peek(0x0100); v != 0x21 {
		t.Fatalf("Peek($0100) = %02X, want 21", v)
	}
	if v, _ := sys.Peek(0x0101); v != 0x46 {
		t.Fatalf("Peek($0101) = %02X, want 46", v)
	}
}

func TestHandleLoadReportsParseError(t *testing.T) {
	sys := kim1.New()
	s := NewServer(sys)

	result := s.handleClientMsg(clientMsg{Type: "load", Format: "pap", Text: "garbage"})
	if result == nil || result.OK {
		t.Fatalf("result = %+v, want a non-OK result", result)
	}
	if result.Message == "" {
		t.Fatalf("expected a non-empty error message")
	}
}

func TestHandleLoadUnknownFormat(t *testing.T) {
	sys := kim1.New()
	s := NewServer(sys)

	result := s.handleClientMsg(clientMsg{Type: "load", Format: "zip", Text: "whatever"})
	if result == nil || result.OK {
		t.Fatalf("result = %+v, want a non-OK result", result)
	}
}
