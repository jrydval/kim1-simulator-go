package kim1

import "testing"

// setupSSTSystem builds a System with a dummy Kbd ROM (NMI vector -> $9000,
// so a serviced NMI lands somewhere observable) and a small RAM program.
func setupSSTSystem(t *testing.T) *System {
	t.Helper()
	s := New()
	kbdROM := dummyROM(map[uint16]uint8{
		0x3FA: 0x00, 0x3FB: 0x90, // NMI vector -> $9000
		0x3FC: 0x00, 0x3FD: 0x02, // reset vector -> $0200
	})
	if err := s.Kbd.LoadROM(kbdROM); err != nil {
		t.Fatal(err)
	}
	// NOP; NOP; NOP at $0200-$0202, a plain RAM program.
	s.Write(0x0200, 0xEA)
	s.Write(0x0201, 0xEA)
	s.Write(0x0202, 0xEA)
	s.Reset()
	return s
}

func TestSSTTriggersNMIAfterRAMInstruction(t *testing.T) {
	s := setupSSTSystem(t)
	s.SST = true

	s.Step() // executes the NOP at $0200, should arm a pending NMI
	if s.CPU.PC != 0x0201 {
		t.Fatalf("PC after first NOP = $%04X, want $0201", s.CPU.PC)
	}
	s.Step() // should service the SST-triggered NMI instead of the next NOP
	if s.CPU.PC != 0x9000 {
		t.Fatalf("PC after SST NMI = $%04X, want $9000 (NMI vector)", s.CPU.PC)
	}
	if !s.CPU.WasInterrupt() {
		t.Fatalf("expected the second Step to have serviced an interrupt")
	}
}

func TestSSTDoesNotTriggerInsideROM(t *testing.T) {
	s := setupSSTSystem(t)
	s.SST = true
	s.CPU.PC = kbdROMStart // pretend we're executing monitor ROM code

	s.Step()
	if s.CPU.PC == 0x9000 {
		t.Fatalf("SST fired an NMI for an instruction fetched from ROM, want no trigger")
	}
}

func TestSSTDoesNotDoubleTriggerOnBRK(t *testing.T) {
	s := setupSSTSystem(t)
	s.SST = true
	s.Write(0x0200, 0x00) // BRK instead of NOP
	// BRK's own IRQ vector -> $A000 (distinct from the NMI vector $9000,
	// so we can tell which one actually fired).
	irqROM := dummyROM(map[uint16]uint8{
		0x3FA: 0x00, 0x3FB: 0x90,
		0x3FC: 0x00, 0x3FD: 0x02,
		0x3FE: 0x00, 0x3FF: 0xA0,
	})
	if err := s.Kbd.LoadROM(irqROM); err != nil {
		t.Fatal(err)
	}
	s.Reset()

	s.Step() // BRK: should go straight to $A000 via its own IRQ handling
	if s.CPU.PC != 0xA000 {
		t.Fatalf("PC after BRK = $%04X, want $A000", s.CPU.PC)
	}

	// If SST had also queued an NMI on top, the *next* step would divert
	// to $9000 (NMI vector) instead of executing whatever's at $A000.
	s.Write(0xA000, 0xEA) // NOP
	s.Step()
	if s.CPU.PC == 0x9000 {
		t.Fatalf("SST double-triggered an NMI on top of BRK's own interrupt entry")
	}
}

func TestSSTOffDoesNotTrigger(t *testing.T) {
	s := setupSSTSystem(t)
	s.SST = false

	s.Step()
	s.Step()
	s.Step()
	if s.CPU.PC != 0x0203 {
		t.Fatalf("PC = $%04X, want $0203 (ran straight through, no NMI)", s.CPU.PC)
	}
}
