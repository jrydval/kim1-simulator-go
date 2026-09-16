// Package bus defines the memory interface the CPU core operates against,
// decoupling instruction execution from any particular memory map.
package bus

// Bus is the seam between cpu.CPU and whatever backs the 6502's address
// space — a flat 64KB RAM for testing, or the full KIM-1 memory-mapped
// system (RAM + RIOT chips) in production use.
type Bus interface {
	Read(addr uint16) uint8
	Write(addr uint16, val uint8)
}

// FlatRAM is a trivial 64KB RAM Bus, used by CPU unit tests and the
// Klaus Dormann functional test harness.
type FlatRAM struct {
	mem [65536]uint8
}

// NewFlatRAM returns a zero-initialized 64KB RAM.
func NewFlatRAM() *FlatRAM {
	return &FlatRAM{}
}

func (r *FlatRAM) Read(addr uint16) uint8 {
	return r.mem[addr]
}

func (r *FlatRAM) Write(addr uint16, val uint8) {
	r.mem[addr] = val
}

// Load copies data into RAM starting at addr, for loading test binaries.
func (r *FlatRAM) Load(addr uint16, data []byte) {
	copy(r.mem[int(addr):], data)
}
