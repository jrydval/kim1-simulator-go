package cpu

import (
	"testing"

	"6502/internal/bus"
)

func newTestCPU() (*CPU, *bus.FlatRAM) {
	ram := bus.NewFlatRAM()
	c := New(ram)
	return c, ram
}

// run loads program at addr, sets PC there, and steps once, returning the
// cycle count.
func run(c *CPU, ram *bus.FlatRAM, addr uint16, program []byte) int {
	ram.Load(addr, program)
	c.PC = addr
	return c.Step()
}

func TestReset(t *testing.T) {
	c, ram := newTestCPU()
	ram.Write(0xFFFC, 0x00)
	ram.Write(0xFFFD, 0x80)
	c.Reset()
	if c.PC != 0x8000 {
		t.Fatalf("PC = $%04X, want $8000", c.PC)
	}
	if !c.flag(FlagI) {
		t.Fatalf("I flag should be set after reset")
	}
	if c.Cycles != 7 {
		t.Fatalf("Cycles = %d, want 7", c.Cycles)
	}
}

func TestLDAImmediateFlags(t *testing.T) {
	c, ram := newTestCPU()
	cycles := run(c, ram, 0x0200, []byte{0xA9, 0x00}) // LDA #$00
	if c.A != 0 || !c.flag(FlagZ) || c.flag(FlagN) {
		t.Fatalf("LDA #$00: A=%02X Z=%v N=%v", c.A, c.flag(FlagZ), c.flag(FlagN))
	}
	if cycles != 2 {
		t.Fatalf("cycles = %d, want 2", cycles)
	}

	cycles = run(c, ram, 0x0200, []byte{0xA9, 0x80}) // LDA #$80
	if c.A != 0x80 || c.flag(FlagZ) || !c.flag(FlagN) {
		t.Fatalf("LDA #$80: A=%02X Z=%v N=%v", c.A, c.flag(FlagZ), c.flag(FlagN))
	}
	_ = cycles
}

func TestLDAZeroPageXWraps(t *testing.T) {
	c, ram := newTestCPU()
	c.X = 0xFF
	ram.Write(0x007F, 0x42) // 0x80 + 0xFF wraps to 0x7F
	cycles := run(c, ram, 0x0200, []byte{0xB5, 0x80}) // LDA $80,X
	if c.A != 0x42 {
		t.Fatalf("A = %02X, want 0x42", c.A)
	}
	if cycles != 4 {
		t.Fatalf("cycles = %d, want 4", cycles)
	}
}

func TestLDAAbsoluteXPageCross(t *testing.T) {
	c, ram := newTestCPU()
	c.X = 0x01
	ram.Write(0x2000, 0x99)
	cycles := run(c, ram, 0x0200, []byte{0xBD, 0xFF, 0x1F}) // LDA $1FFF,X -> $2000
	if c.A != 0x99 {
		t.Fatalf("A = %02X, want 0x99", c.A)
	}
	if cycles != 5 {
		t.Fatalf("cycles = %d, want 5 (page cross)", cycles)
	}

	cycles = run(c, ram, 0x0200, []byte{0xBD, 0x00, 0x20}) // LDA $2000,X -> $2001, no cross
	if cycles != 4 {
		t.Fatalf("cycles = %d, want 4 (no page cross)", cycles)
	}
}

func TestSTAAbsoluteXNoPageCrossBonus(t *testing.T) {
	// STA abs,X is always 5 cycles regardless of page crossing.
	c, ram := newTestCPU()
	c.A = 0x55
	c.X = 0x01
	cycles := run(c, ram, 0x0200, []byte{0x9D, 0x00, 0x20}) // STA $2000,X
	if ram.Read(0x2001) != 0x55 {
		t.Fatalf("mem[$2001] = %02X, want 0x55", ram.Read(0x2001))
	}
	if cycles != 5 {
		t.Fatalf("cycles = %d, want 5", cycles)
	}
}

func TestIndexedIndirect(t *testing.T) {
	c, ram := newTestCPU()
	c.X = 0x04
	ram.Write(0x0024, 0x00) // ptr low at zp+X
	ram.Write(0x0025, 0x03) // ptr high
	ram.Write(0x0300, 0x77)
	run(c, ram, 0x0200, []byte{0xA1, 0x20}) // LDA ($20,X)
	if c.A != 0x77 {
		t.Fatalf("A = %02X, want 0x77", c.A)
	}
}

func TestIndirectIndexedPageCross(t *testing.T) {
	c, ram := newTestCPU()
	c.Y = 0x01
	ram.Write(0x0020, 0xFF) // ptr low
	ram.Write(0x0021, 0x1F) // ptr high -> base $1FFF, +Y=1 -> $2000
	ram.Write(0x2000, 0x33)
	cycles := run(c, ram, 0x0200, []byte{0xB1, 0x20}) // LDA ($20),Y
	if c.A != 0x33 {
		t.Fatalf("A = %02X, want 0x33", c.A)
	}
	if cycles != 6 {
		t.Fatalf("cycles = %d, want 6 (5 base + 1 page cross)", cycles)
	}
}

func TestZeroPageIndirectPointerWrapsWithinPage(t *testing.T) {
	// (zp,X) pointer bytes must wrap within the zero page, not into page 1.
	c, ram := newTestCPU()
	ram.Write(0x00FF, 0x34) // low byte of pointer at $FF
	ram.Write(0x0000, 0x12) // high byte wraps to $00, not $100
	ram.Write(0x1234, 0x99)
	run(c, ram, 0x0200, []byte{0xA1, 0xFF}) // LDA ($FF,X) with X=0
	if c.A != 0x99 {
		t.Fatalf("A = %02X, want 0x99 (zero-page pointer wrap)", c.A)
	}
}

func TestJMPIndirectPageWrapBug(t *testing.T) {
	c, ram := newTestCPU()
	ram.Write(0x02FF, 0x00) // low byte of target
	ram.Write(0x0200, 0x80) // hardware bug: high byte fetched from $0200, not $0300
	ram.Write(0x0300, 0xFF) // decoy, should NOT be used
	run(c, ram, 0x1000, []byte{0x6C, 0xFF, 0x02}) // JMP ($02FF)
	if c.PC != 0x8000 {
		t.Fatalf("PC = $%04X, want $8000 (page-wrap bug)", c.PC)
	}
}

func TestADCBinaryOverflow(t *testing.T) {
	c, ram := newTestCPU()
	c.A = 0x50
	run(c, ram, 0x0200, []byte{0x69, 0x50}) // ADC #$50 -> 0xA0, signed overflow
	if c.A != 0xA0 {
		t.Fatalf("A = %02X, want 0xA0", c.A)
	}
	if !c.flag(FlagV) {
		t.Fatalf("V flag should be set (positive+positive=negative)")
	}
	if !c.flag(FlagN) {
		t.Fatalf("N flag should be set")
	}
	if c.flag(FlagC) {
		t.Fatalf("C flag should be clear")
	}
}

func TestADCCarryOut(t *testing.T) {
	c, ram := newTestCPU()
	c.A = 0xFF
	run(c, ram, 0x0200, []byte{0x69, 0x01}) // ADC #$01 -> 0x00, carry out
	if c.A != 0x00 {
		t.Fatalf("A = %02X, want 0x00", c.A)
	}
	if !c.flag(FlagC) {
		t.Fatalf("C flag should be set")
	}
	if !c.flag(FlagZ) {
		t.Fatalf("Z flag should be set")
	}
}

func TestADCDecimalMode(t *testing.T) {
	c, ram := newTestCPU()
	c.setFlag(FlagD, true)
	c.A = 0x58
	run(c, ram, 0x0200, []byte{0x69, 0x46}) // ADC #$46 (decimal) -> 58+46=104 -> $04, C set
	if c.A != 0x04 {
		t.Fatalf("A = %02X, want 0x04 (BCD 58+46=104)", c.A)
	}
	if !c.flag(FlagC) {
		t.Fatalf("C flag should be set (BCD result > 99)")
	}
}

func TestSBCBinary(t *testing.T) {
	c, ram := newTestCPU()
	c.A = 0x50
	c.setFlag(FlagC, true) // no borrow
	run(c, ram, 0x0200, []byte{0xE9, 0x30}) // SBC #$30 -> 0x20
	if c.A != 0x20 {
		t.Fatalf("A = %02X, want 0x20", c.A)
	}
	if !c.flag(FlagC) {
		t.Fatalf("C flag should be set (no borrow)")
	}
}

func TestCompareFlags(t *testing.T) {
	c, ram := newTestCPU()
	c.A = 0x10
	run(c, ram, 0x0200, []byte{0xC9, 0x10}) // CMP #$10 -> equal
	if !c.flag(FlagZ) || !c.flag(FlagC) {
		t.Fatalf("CMP equal: Z=%v C=%v, want both true", c.flag(FlagZ), c.flag(FlagC))
	}

	c.A = 0x10
	run(c, ram, 0x0200, []byte{0xC9, 0x20}) // CMP #$20 -> A < M
	if c.flag(FlagC) {
		t.Fatalf("CMP A<M: C should be clear")
	}
}

func TestASLMemoryAndAccumulator(t *testing.T) {
	c, ram := newTestCPU()
	ram.Write(0x0010, 0x81)
	run(c, ram, 0x0200, []byte{0x06, 0x10}) // ASL $10
	if ram.Read(0x0010) != 0x02 {
		t.Fatalf("mem[$10] = %02X, want 0x02", ram.Read(0x0010))
	}
	if !c.flag(FlagC) {
		t.Fatalf("C flag should be set from bit 7")
	}

	c.A = 0x81
	run(c, ram, 0x0200, []byte{0x0A}) // ASL A
	if c.A != 0x02 {
		t.Fatalf("A = %02X, want 0x02", c.A)
	}
}

func TestBranchTakenAndPageCross(t *testing.T) {
	c, ram := newTestCPU()
	c.setFlag(FlagZ, true)
	// BEQ at $02FC: PC after fetching the operand is $02FE, so offset +2
	// lands at $0300 — crossing from page $02 to page $03.
	cycles := run(c, ram, 0x02FC, []byte{0xF0, 0x02}) // BEQ +2 -> target $0300
	if c.PC != 0x0300 {
		t.Fatalf("PC = $%04X, want $0300", c.PC)
	}
	if cycles != 4 {
		t.Fatalf("cycles = %d, want 4 (2 base + 1 taken + 1 page-cross)", cycles)
	}
}

func TestBranchNotTaken(t *testing.T) {
	c, ram := newTestCPU()
	c.setFlag(FlagZ, false)
	cycles := run(c, ram, 0x0200, []byte{0xF0, 0x10}) // BEQ, not taken
	if c.PC != 0x0202 {
		t.Fatalf("PC = $%04X, want $0202", c.PC)
	}
	if cycles != 2 {
		t.Fatalf("cycles = %d, want 2", cycles)
	}
}

func TestStackPushPull(t *testing.T) {
	c, ram := newTestCPU()
	c.SP = 0xFD
	c.A = 0x42
	run(c, ram, 0x0200, []byte{0x48}) // PHA
	if ram.Read(0x01FD) != 0x42 {
		t.Fatalf("stack[$1FD] = %02X, want 0x42", ram.Read(0x01FD))
	}
	if c.SP != 0xFC {
		t.Fatalf("SP = %02X, want 0xFC", c.SP)
	}
	c.A = 0
	run(c, ram, 0x0200, []byte{0x68}) // PLA
	if c.A != 0x42 || c.SP != 0xFD {
		t.Fatalf("PLA: A=%02X SP=%02X, want A=42 SP=FD", c.A, c.SP)
	}
}

func TestJSRRTS(t *testing.T) {
	c, ram := newTestCPU()
	c.SP = 0xFF
	ram.Load(0x0300, []byte{0x60}) // RTS at subroutine
	run(c, ram, 0x0200, []byte{0x20, 0x00, 0x03}) // JSR $0300
	if c.PC != 0x0300 {
		t.Fatalf("PC after JSR = $%04X, want $0300", c.PC)
	}
	c.Step() // RTS
	if c.PC != 0x0203 {
		t.Fatalf("PC after RTS = $%04X, want $0203", c.PC)
	}
}

func TestBRKandRTI(t *testing.T) {
	c, ram := newTestCPU()
	c.SP = 0xFF
	ram.Write(0xFFFE, 0x00)
	ram.Write(0xFFFF, 0x90) // IRQ/BRK vector -> $9000
	run(c, ram, 0x0200, []byte{0x00, 0xEA}) // BRK (padding byte 0xEA skipped)
	if c.PC != 0x9000 {
		t.Fatalf("PC after BRK = $%04X, want $9000", c.PC)
	}
	if !c.flag(FlagI) {
		t.Fatalf("I flag should be set after BRK")
	}
	ram.Load(0x9000, []byte{0x40}) // RTI
	c.PC = 0x9000
	c.Step()
	if c.PC != 0x0202 {
		t.Fatalf("PC after RTI = $%04X, want $0202", c.PC)
	}
}

func TestIllegalOpcodePanics(t *testing.T) {
	c, ram := newTestCPU()
	ram.Load(0x0200, []byte{0x02}) // undefined opcode
	c.PC = 0x0200
	defer func() {
		if recover() == nil {
			t.Fatalf("expected panic on illegal opcode")
		}
	}()
	c.Step()
}
