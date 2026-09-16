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

func TestWebSocketUpgradeAndBroadcast(t *testing.T) {
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

	// Give the server a moment to register the client, then force a
	// broadcast rather than waiting on the 30Hz ticker.
	time.Sleep(20 * time.Millisecond)
	s.broadcastState()

	var msg stateMsg
	if err := wsjson.Read(ctx, conn, &msg); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if msg.Type != "state" {
		t.Fatalf("Type = %q, want %q", msg.Type, "state")
	}
}

func TestWebSocketKeyMessageReachesKeypad(t *testing.T) {
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

	if err := wsjson.Write(ctx, conn, clientMsg{Type: "key", Row: 1, Col: 2, Down: true}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		bits := sys.Keypad.ColumnBits(1)
		s.mu.Unlock()
		if bits&(1<<2) == 0 {
			return // key registered as pressed
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("key press did not reach Keypad within timeout")
}

// TestCPUPanicDoesNotCrashServer is a regression test: an unrecovered
// panic in the CPU-stepping goroutine (e.g. an illegal opcode reached by
// GO-ing into uninitialized memory) used to kill the whole process
// silently, which from the browser looked exactly like the UI freezing.
func TestCPUPanicDoesNotCrashServer(t *testing.T) {
	sys := kim1.New()
	sys.Reset()
	sys.Write(0x0200, 0x02) // undefined opcode
	sys.CPU.PC = 0x0200

	s := NewServer(sys)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.RunCPU(ctx)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		halted := s.halted
		s.mu.Unlock()
		if halted {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	s.mu.Lock()
	halted, reason := s.halted, s.haltReason
	s.mu.Unlock()
	if !halted {
		t.Fatalf("server did not record the halt within timeout")
	}
	if reason == "" {
		t.Fatalf("haltReason is empty")
	}

	// Confirm the CPU has actually stopped advancing, not just flagged.
	s.mu.Lock()
	before := sys.CPU.Cycles
	s.mu.Unlock()
	time.Sleep(100 * time.Millisecond)
	s.mu.Lock()
	after := sys.CPU.Cycles
	s.mu.Unlock()
	if before != after {
		t.Fatalf("CPU kept running after halt: cycles %d -> %d", before, after)
	}

	// Reset (as the RS button does) should clear the halt and let
	// execution resume.
	s.handleClientMsg(clientMsg{Type: "reset"})
	time.Sleep(100 * time.Millisecond)
	s.mu.Lock()
	stillHalted := s.halted
	s.mu.Unlock()
	if stillHalted {
		t.Fatalf("halt was not cleared by reset")
	}
}
