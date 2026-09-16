// Package riot models the MOS 6530 RIOT (RAM-I/O-Timer) chip: 1KB of mask
// ROM, 64 bytes of RAM, two 8-bit bidirectional I/O ports with data
// direction registers, and an interval timer. The KIM-1 uses two of these
// (see internal/kim1), one for the keypad/display and one for the
// TTY/cassette interface.
package riot

import "fmt"

const RAMSize = 64
const ROMSize = 1024

// Port is one 8-bit bidirectional I/O port with a data direction register.
// A DDR bit of 1 makes the corresponding pin an output driven by the last
// value written; a DDR bit of 0 makes it an input, whose value on read is
// supplied by InputFunc (e.g. a keypad row scan), if set.
type Port struct {
	data uint8
	ddr  uint8

	// InputFunc supplies externally-driven input bit values, called on
	// every Read. May be nil, in which case input bits read as 0.
	InputFunc func() uint8
}

func (p *Port) Write(v uint8)     { p.data = v }
func (p *Port) WriteDDR(v uint8)  { p.ddr = v }
func (p *Port) ReadDDR() uint8    { return p.ddr }
func (p *Port) OutputData() uint8 { return p.data } // raw latch, ignoring DDR — for display/debug

func (p *Port) Read() uint8 {
	out := p.data & p.ddr
	var in uint8
	if p.InputFunc != nil {
		in = p.InputFunc() &^ p.ddr
	}
	return out | in
}

// RIOT is one MOS 6530 chip.
type RIOT struct {
	RAM [RAMSize]uint8
	ROM [ROMSize]uint8

	PortA Port
	PortB Port
	Timer Timer
}

// New returns a RIOT with zeroed RAM/ROM/ports.
func New() *RIOT {
	return &RIOT{}
}

// LoadROM installs a 1KB ROM image. It returns an error if data is not
// exactly ROMSize bytes.
func (r *RIOT) LoadROM(data []byte) error {
	if len(data) != ROMSize {
		return fmt.Errorf("riot: ROM must be exactly %d bytes, got %d", ROMSize, len(data))
	}
	copy(r.ROM[:], data)
	return nil
}

func (r *RIOT) ReadRAM(offset uint8) uint8      { return r.RAM[offset&(RAMSize-1)] }
func (r *RIOT) WriteRAM(offset uint8, v uint8)  { r.RAM[offset&(RAMSize-1)] = v }
func (r *RIOT) ReadROM(offset uint16) uint8     { return r.ROM[offset&(ROMSize-1)] }

// TickTimer advances this chip's timer and asserts IRQ-worthiness via the
// return value (true if the timer just underflowed with interrupts
// enabled and the flag hasn't been serviced yet) so callers (kim1.System)
// can raise CPU.IRQ appropriately.
func (r *RIOT) TickTimer(cycles int) {
	r.Timer.Tick(cycles)
}
