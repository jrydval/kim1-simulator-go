package cpu

import "testing"

// instrumentedBus is a minimal second bus.Bus implementation (distinct
// from bus.FlatRAM) used to prove that cpu.CPU is driven entirely through
// the Bus interface and holds no KIM-1-specific (or any other
// memory-map-specific) knowledge.
type instrumentedBus struct {
	mem   [65536]uint8
	reads int
}

func (b *instrumentedBus) Read(addr uint16) uint8 {
	b.reads++
	return b.mem[addr]
}

func (b *instrumentedBus) Write(addr uint16, v uint8) {
	b.mem[addr] = v
}

func TestCPUIsPolymorphicOverBus(t *testing.T) {
	b := &instrumentedBus{}
	b.mem[0x0200] = 0xA9 // LDA #$99
	b.mem[0x0201] = 0x99

	c := New(b)
	c.PC = 0x0200
	c.Step()

	if c.A != 0x99 {
		t.Fatalf("A = %02X, want 0x99", c.A)
	}
	if b.reads == 0 {
		t.Fatalf("expected CPU to read through the Bus interface")
	}
}
