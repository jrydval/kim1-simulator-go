package kim1

import "testing"

func TestPeekRAMAndROM(t *testing.T) {
	s := New()
	s.Write(0x0200, 0x42)
	s.Kbd.ROM[0x3FA] = 0x99

	if v, ok := s.Peek(0x0200); !ok || v != 0x42 {
		t.Fatalf("Peek($0200) = %02X, %v; want 42, true", v, ok)
	}
	if v, ok := s.Peek(0x1FFA); !ok || v != 0x99 {
		t.Fatalf("Peek($1FFA) = %02X, %v; want 99, true", v, ok)
	}
	if v, ok := s.Peek(0xFFFA); !ok || v != 0x99 {
		t.Fatalf("Peek($FFFA) = %02X, %v; want 99, true (aliased vector)", v, ok)
	}
}

func TestPeekIOWindowAndUnmappedAreNotOK(t *testing.T) {
	s := New()
	for _, addr := range []uint16{0x1700, 0x1747, 0x177F, 0x0400, 0x2000, 0xFF00} {
		if _, ok := s.Peek(addr); ok {
			t.Fatalf("Peek(%04X) ok = true, want false", addr)
		}
	}
}

func TestRecentPCsNewestFirstAndCapped(t *testing.T) {
	s := New()
	// A run of NOPs at $0200.
	for i := 0; i < 20; i++ {
		s.Write(uint16(0x0200+i), 0xEA)
	}
	s.CPU.PC = 0x0200
	for i := 0; i < 3; i++ {
		s.Step()
	}
	got := s.RecentPCs()
	if len(got) != 3 || got[0] != 0x0202 || got[1] != 0x0201 || got[2] != 0x0200 {
		t.Fatalf("RecentPCs after 3 NOPs = %04X, want [0202 0201 0200]", got)
	}

	for i := 0; i < 12; i++ {
		s.Step()
	}
	got = s.RecentPCs()
	if len(got) != PCHistoryLen || got[0] != 0x020E || got[PCHistoryLen-1] != 0x0205 {
		t.Fatalf("RecentPCs after 15 NOPs = %04X, want 10 entries from 020E down to 0205", got)
	}
}

func TestRecentPCsSkipsInterruptResponses(t *testing.T) {
	s := New()
	s.Write(0x0200, 0xEA)
	s.CPU.PC = 0x0200
	s.Step()
	s.CPU.NMI()
	s.Step() // services the NMI: not an executed instruction
	if got := s.RecentPCs(); len(got) != 1 || got[0] != 0x0200 {
		t.Fatalf("RecentPCs = %04X, want only [0200] (interrupt response isn't an instruction)", got)
	}
}
