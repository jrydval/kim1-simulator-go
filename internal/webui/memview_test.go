package webui

import (
	"testing"

	"6502/internal/kim1"
)

func TestMemWriteAndWindow(t *testing.T) {
	sys := kim1.New()
	s := NewServer(sys)

	s.handleClientMsg(clientMsg{Type: "memwrite", Addr: 0x0203, Val: 0x42})
	s.handleClientMsg(clientMsg{Type: "memview", Addr: 0x0203}) // must align down to the 256-byte page

	if s.memViewAddr != 0x0200 {
		t.Fatalf("memViewAddr = %04X, want 0200 (aligned to the window size)", s.memViewAddr)
	}
	w := s.memWindow()
	if len(w) != memViewBytes || w[3] != 0x42 {
		t.Fatalf("window[3] = %d (len %d), want 66 (0x42)", w[3], len(w))
	}
}

func TestMemWriteIgnoredOutsidePeekableSpace(t *testing.T) {
	sys := kim1.New()
	s := NewServer(sys)

	s.handleClientMsg(clientMsg{Type: "memwrite", Addr: 0x1741, Val: 0xFF}) // Kbd Port A DDR
	if got := sys.Kbd.PortA.ReadDDR(); got != 0 {
		t.Fatalf("memwrite into an I/O register window changed DDR to %02X, want it ignored", got)
	}
}

func TestMemWindowMarksUnviewableAddresses(t *testing.T) {
	sys := kim1.New()
	s := NewServer(sys)
	s.handleClientMsg(clientMsg{Type: "memview", Addr: 0x1700})

	w := s.memWindow()
	if w[0] != -1 || w[0x80] < 0 {
		t.Fatalf("window[$00]=%d window[$80]=%d; want -1 for the I/O window and >=0 for RIOT RAM", w[0], w[0x80])
	}
}
