package kim1

import "testing"

func TestSystemRAM(t *testing.T) {
	s := New()
	s.Write(0x0042, 0x99)
	if got := s.Read(0x0042); got != 0x99 {
		t.Fatalf("Read($0042) = %02X, want 0x99", got)
	}
}

func TestAppRIOTIOWindow(t *testing.T) {
	s := New()
	s.Write(0x1701, 0xFF) // App PADD: all output
	s.Write(0x1700, 0x5A) // App PAD data
	if got := s.Read(0x1700); got != 0x5A {
		t.Fatalf("Read($1700) = %02X, want 0x5A", got)
	}
	if got := s.Read(0x1701); got != 0xFF {
		t.Fatalf("Read($1701) DDR readback = %02X, want 0xFF", got)
	}
}

func TestKbdRIOTIOWindow(t *testing.T) {
	s := New()
	s.Write(0x1743, 0x0F) // Kbd PBDD
	s.Write(0x1742, 0xAA) // Kbd PBD data
	if got := s.Read(0x1743); got != 0x0F {
		t.Fatalf("Read($1743) = %02X, want 0x0F", got)
	}
	if got := s.Read(0x1742); got != 0x0A { // only low nibble is output
		t.Fatalf("Read($1742) = %02X, want 0x0A", got)
	}
}

func TestIOWindowMirroring(t *testing.T) {
	// Each RIOT decodes only the low 3 address bits within its window,
	// so $1708 (offset 8) must alias back to register 0 (Port A data),
	// same as $1700.
	s := New()
	s.Write(0x1701, 0xFF)
	s.Write(0x1708, 0x33)
	if got := s.Read(0x1700); got != 0x33 {
		t.Fatalf("mirrored write via $1708 not visible at $1700: got %02X", got)
	}
}

func TestTimerThroughIOWindow(t *testing.T) {
	s := New()
	s.Write(0x1704, 10) // App RIOT: timer, divide-by-1, value=10
	for i := 0; i < 10; i++ {
		if v := s.Read(0x1706); v != uint8(10-i) { // even offset reads timer value
			t.Fatalf("timer value = %d, want %d", v, 10-i)
		}
		s.App.TickTimer(1)
	}
}

func TestAppAndKbdRAMWindows(t *testing.T) {
	s := New()
	s.Write(0x1780, 0x11) // App RIOT RAM
	s.Write(0x17C0, 0x22) // Kbd RIOT RAM
	if got := s.Read(0x1780); got != 0x11 {
		t.Fatalf("App RAM = %02X, want 0x11", got)
	}
	if got := s.Read(0x17C0); got != 0x22 {
		t.Fatalf("Kbd RAM = %02X, want 0x22", got)
	}
	// The two RIOTs' RAM must be independent.
	if got := s.Read(0x1781); got == 0x22 {
		t.Fatalf("App and Kbd RAM windows appear aliased")
	}
}

// dummyROM builds a 1024-byte ROM image with a single byte set at the
// given offset, for exercising the ROM decode/vector-aliasing logic
// without needing a real (copyrighted) KIM-1 ROM dump.
func dummyROM(patches map[uint16]uint8) []byte {
	rom := make([]byte, 1024)
	for off, v := range patches {
		rom[off] = v
	}
	return rom
}

func TestROMWindowsAndVectorAlias(t *testing.T) {
	s := New()
	appROM := dummyROM(map[uint16]uint8{0x000: 0xEA}) // NOP at $1800
	if err := s.App.LoadROM(appROM); err != nil {
		t.Fatalf("LoadROM(App): %v", err)
	}
	// Kbd ROM offset $3FC/$3FD = absolute $1FFC/$1FFD = reset vector entry.
	kbdROM := dummyROM(map[uint16]uint8{0x3FC: 0x00, 0x3FD: 0x90})
	if err := s.Kbd.LoadROM(kbdROM); err != nil {
		t.Fatalf("LoadROM(Kbd): %v", err)
	}

	if got := s.Read(0x1800); got != 0xEA {
		t.Fatalf("Read($1800) = %02X, want 0xEA", got)
	}
	if got := s.Read(0xFFFC); got != 0x00 || s.Read(0xFFFD) != 0x90 {
		t.Fatalf("hardware reset vector not aliased to Kbd ROM: lo=%02X hi=%02X", s.Read(0xFFFC), s.Read(0xFFFD))
	}

	s.Reset()
	if s.CPU.PC != 0x9000 {
		t.Fatalf("PC after Reset = $%04X, want $9000", s.CPU.PC)
	}
}

func TestLoadROMRejectsWrongSize(t *testing.T) {
	s := New()
	if err := s.App.LoadROM(make([]byte, 10)); err == nil {
		t.Fatalf("expected error for wrong-size ROM")
	}
}

func TestEndToEndProgramThroughSystem(t *testing.T) {
	s := New()
	kbdROM := dummyROM(map[uint16]uint8{0x3FC: 0x00, 0x3FD: 0x02}) // reset vector -> $0200
	if err := s.Kbd.LoadROM(kbdROM); err != nil {
		t.Fatal(err)
	}
	// LDA #$42; STA $1700 (App Port A data); LDX $1701 (its DDR, still 0)
	prog := []byte{0xA9, 0x42, 0x8D, 0x00, 0x17, 0xAE, 0x01, 0x17}
	for i, b := range prog {
		s.Write(0x0200+uint16(i), b)
	}
	s.Reset()
	if s.CPU.PC != 0x0200 {
		t.Fatalf("PC after Reset = $%04X, want $0200", s.CPU.PC)
	}
	s.Step() // LDA
	s.Step() // STA
	s.Step() // LDX
	if s.CPU.A != 0x42 {
		t.Fatalf("A = %02X, want 0x42", s.CPU.A)
	}
	if s.CPU.X != 0x00 {
		t.Fatalf("X = %02X, want 0x00 (DDR still all-input)", s.CPU.X)
	}
	if got := s.Read(0x1700); got != 0x00 {
		// PAD reads 0 because DDR is still all-input (no InputFunc set),
		// even though 0x42 was latched — this documents current default
		// behavior before Milestone 6 wires a keypad InputFunc.
		t.Fatalf("Read($1700) = %02X, want 0x00 with DDR=0 and no InputFunc", got)
	}
}
