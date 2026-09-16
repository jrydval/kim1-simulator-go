// Package cpu implements a MOS 6502 CPU core. It knows nothing about any
// particular memory map — it operates entirely through the bus.Bus
// interface, so the same core drives both a flat 64KB RAM test harness
// and the full KIM-1 memory-mapped system.
package cpu

import "6502/internal/bus"

// Status flag bits within the P register.
const (
	FlagC uint8 = 1 << 0 // Carry
	FlagZ uint8 = 1 << 1 // Zero
	FlagI uint8 = 1 << 2 // Interrupt disable
	FlagD uint8 = 1 << 3 // Decimal mode
	FlagB uint8 = 1 << 4 // Break (only meaningful in the byte pushed to the stack)
	Flag5 uint8 = 1 << 5 // Unused, always reads as 1
	FlagV uint8 = 1 << 6 // Overflow
	FlagN uint8 = 1 << 7 // Negative
)

const stackBase uint16 = 0x0100

// resetVector, irqVector, nmiVector are the fixed hardware vector addresses.
const (
	nmiVector   uint16 = 0xFFFA
	resetVector uint16 = 0xFFFC
	irqVector   uint16 = 0xFFFE
)

// CPU holds all 6502 registers and drives instruction execution against a
// Bus.
type CPU struct {
	A, X, Y uint8
	SP      uint8
	PC      uint16
	P       uint8

	Bus bus.Bus

	// Cycles is the running total of cycles executed since power-on/reset,
	// used by callers (e.g. kim1.System) to tick peripherals in step.
	Cycles uint64

	irqPending           bool
	nmiPending           bool
	lastStepWasInterrupt bool
}

// New creates a CPU driving the given Bus. Call Reset before running it.
func New(b bus.Bus) *CPU {
	return &CPU{Bus: b}
}

func (c *CPU) read(addr uint16) uint8     { return c.Bus.Read(addr) }
func (c *CPU) write(addr uint16, v uint8) { c.Bus.Write(addr, v) }
func (c *CPU) read16(addr uint16) uint16 {
	lo := uint16(c.read(addr))
	hi := uint16(c.read(addr + 1))
	return hi<<8 | lo
}

// readBug16 reproduces the classic 6502 indirect-addressing page-wrap bug:
// if the low byte of the pointer is 0xFF, the high byte is fetched from the
// start of the same page rather than the next page.
func (c *CPU) readBug16(addr uint16) uint16 {
	lo := uint16(c.read(addr))
	hiAddr := (addr & 0xFF00) | uint16(uint8(addr)+1)
	hi := uint16(c.read(hiAddr))
	return hi<<8 | lo
}

func (c *CPU) push(v uint8) {
	c.write(stackBase+uint16(c.SP), v)
	c.SP--
}

func (c *CPU) pop() uint8 {
	c.SP++
	return c.read(stackBase + uint16(c.SP))
}

func (c *CPU) push16(v uint16) {
	c.push(uint8(v >> 8))
	c.push(uint8(v))
}

func (c *CPU) pop16() uint16 {
	lo := uint16(c.pop())
	hi := uint16(c.pop())
	return hi<<8 | lo
}

func (c *CPU) setFlag(flag uint8, on bool) {
	if on {
		c.P |= flag
	} else {
		c.P &^= flag
	}
}

func (c *CPU) flag(flag uint8) bool { return c.P&flag != 0 }

func (c *CPU) setZN(v uint8) {
	c.setFlag(FlagZ, v == 0)
	c.setFlag(FlagN, v&0x80 != 0)
}

// Reset performs a power-on/reset sequence: loads PC from the reset vector,
// sets I, clears B, and consumes 7 cycles (the real 6502's reset timing).
func (c *CPU) Reset() {
	c.SP -= 3
	c.setFlag(FlagI, true)
	c.PC = c.read16(resetVector)
	c.Cycles += 7
}

// IRQ requests a maskable interrupt; it takes effect on the next Step call
// if the interrupt disable flag is clear.
func (c *CPU) IRQ() { c.irqPending = true }

// NMI requests a non-maskable interrupt; it always takes effect on the next
// Step call.
func (c *CPU) NMI() { c.nmiPending = true }

func (c *CPU) handleInterrupt(vector uint16, brk bool) int {
	c.push16(c.PC)
	flags := c.P | Flag5
	if brk {
		flags |= FlagB
	} else {
		flags &^= FlagB
	}
	c.push(flags)
	c.setFlag(FlagI, true)
	c.PC = c.read16(vector)
	return 7
}

// Step executes exactly one instruction (or services a pending interrupt)
// and returns the number of cycles consumed.
func (c *CPU) Step() int {
	c.lastStepWasInterrupt = false
	if c.nmiPending {
		c.nmiPending = false
		n := c.handleInterrupt(nmiVector, false)
		c.Cycles += uint64(n)
		c.lastStepWasInterrupt = true
		return n
	}
	if c.irqPending {
		c.irqPending = false
		if !c.flag(FlagI) {
			n := c.handleInterrupt(irqVector, false)
			c.Cycles += uint64(n)
			c.lastStepWasInterrupt = true
			return n
		}
	}

	n := c.execute()
	c.Cycles += uint64(n)
	return n
}

// WasInterrupt reports whether the most recent Step call serviced a
// pending IRQ/NMI (via IRQ/NMI) rather than executing a normal
// instruction. Note this is false for BRK, which is a normal
// instruction fetch that happens to also enter the interrupt handler.
func (c *CPU) WasInterrupt() bool { return c.lastStepWasInterrupt }
