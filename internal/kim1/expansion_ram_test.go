package kim1

import "testing"

func TestExpansionRAMDisabledByDefault(t *testing.T) {
	s := New()
	s.Write(0x2000, 0x42)
	if got := s.Read(0x2000); got != 0 {
		t.Fatalf("Read($2000) without EnableExpansionRAM = %02X, want 0 (open bus, write ignored)", got)
	}
	if _, ok := s.Peek(0x2000); ok {
		t.Fatalf("Peek($2000) without EnableExpansionRAM: ok = true, want false")
	}
}

func TestExpansionRAMReadWrite(t *testing.T) {
	s := New()
	s.EnableExpansionRAM()

	s.Write(0x2000, 0x11)
	s.Write(0xFFF9, 0x22) // top of the window, one byte below the vectors
	if got := s.Read(0x2000); got != 0x11 {
		t.Fatalf("Read($2000) = %02X, want 11", got)
	}
	if got := s.Read(0xFFF9); got != 0x22 {
		t.Fatalf("Read($FFF9) = %02X, want 22", got)
	}
	if v, ok := s.Peek(0x8000); !ok || v != 0 {
		t.Fatalf("Peek($8000) = %02X, %v; want 00, true", v, ok)
	}
}

func TestExpansionRAMDoesNotShadowVectors(t *testing.T) {
	s := New()
	s.EnableExpansionRAM()
	s.Kbd.ROM[0x3FA] = 0x99 // $1FFA, aliased to by $FFFA

	s.Write(0xFFFA, 0x55) // a write to the vector address must be a no-op
	if got := s.Read(0xFFFA); got != 0x99 {
		t.Fatalf("Read($FFFA) with expansion RAM enabled = %02X, want 99 (still aliased to Kbd ROM)", got)
	}
}
