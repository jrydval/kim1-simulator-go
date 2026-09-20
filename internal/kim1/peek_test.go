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
