// Package kim1 wires a 6502 CPU, system RAM, and two MOS 6530 RIOT chips
// into the full KIM-1 memory map, implementing bus.Bus so the same cpu.CPU
// core used for the flat-RAM test harness also drives the real system.
package kim1

import (
	"fmt"
	"os"

	"6502/internal/cpu"
	"6502/internal/riot"
)

// System is a complete KIM-1: CPU + RAM + the two RIOT chips.
type System struct {
	CPU *cpu.CPU
	RAM [ramEnd - ramStart + 1]uint8

	App *riot.RIOT // 6530-003: user application I/O connector
	Kbd *riot.RIOT // 6530-002: keypad, display, TTY, cassette

	Keypad  *Keypad
	Display *Display

	// DebugIO, if set, is called for every write to either RIOT's I/O
	// register window (post address-decode), for diagnosing real ROM
	// interoperability issues.
	DebugIO func(chip string, offset uint16, v uint8)
}

// New returns a System with empty RAM/ROM. Load ROM images with
// LoadAppROM/LoadKbdROM before calling Reset.
func New() *System {
	s := &System{
		App:     riot.New(),
		Kbd:     riot.New(),
		Keypad:  NewKeypad(),
		Display: NewDisplay(),
	}
	s.Kbd.PortA.InputFunc = s.keypadColumnInput
	s.CPU = cpu.New(s)
	return s
}

// LoadAppROM installs the application RIOT's (6530-003, $1800-$1BFF) 1KB
// ROM image from a user-supplied file.
func (s *System) LoadAppROM(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("kim1: reading application RIOT ROM: %w", err)
	}
	if err := s.App.LoadROM(data); err != nil {
		return fmt.Errorf("kim1: %w (see testdata/README.md for sourcing instructions)", err)
	}
	return nil
}

// LoadKbdROM installs the keypad/display RIOT's (6530-002, $1C00-$1FFF)
// 1KB ROM image, including the hardware vector entry points at
// $1FFA-$1FFF, from a user-supplied file.
func (s *System) LoadKbdROM(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("kim1: reading keyboard/display RIOT ROM: %w", err)
	}
	if err := s.Kbd.LoadROM(data); err != nil {
		return fmt.Errorf("kim1: %w (see testdata/README.md for sourcing instructions)", err)
	}
	return nil
}

// Reset performs the CPU reset sequence, fetching PC from the aliased
// hardware reset vector ($FFFC/$FFFD -> $1FFC/$1FFD in Kbd ROM).
func (s *System) Reset() {
	s.CPU.Reset()
}

// Step executes one CPU instruction and ticks both RIOT timers by the
// number of cycles it consumed.
func (s *System) Step() int {
	cycles := s.CPU.Step()
	s.App.TickTimer(cycles)
	s.Kbd.TickTimer(cycles)
	return cycles
}

// Read implements bus.Bus.
func (s *System) Read(addr uint16) uint8 {
	switch {
	case addr <= ramEnd:
		return s.RAM[addr-ramStart]
	case addr >= appIOStart && addr <= appIOEnd:
		return readIO(s.App, addr-appIOStart)
	case addr >= kbdIOStart && addr <= kbdIOEnd:
		return readIO(s.Kbd, addr-kbdIOStart)
	case addr >= appRAMStart && addr <= appRAMEnd:
		return s.App.ReadRAM(uint8(addr - appRAMStart))
	case addr >= kbdRAMStart && addr <= kbdRAMEnd:
		return s.Kbd.ReadRAM(uint8(addr - kbdRAMStart))
	case addr >= appROMStart && addr <= appROMEnd:
		return s.App.ReadROM(addr - appROMStart)
	case addr >= kbdROMStart && addr <= kbdROMEnd:
		return s.Kbd.ReadROM(addr - kbdROMStart)
	case addr >= hwVectorStart:
		// riot.ReadROM masks to the low 10 bits internally, so the raw
		// $FFFA-$FFFF address already lands on the same ROM cells as
		// $1FFA-$1FFF without needing an explicit offset subtraction.
		return s.Kbd.ReadROM(addr)
	default:
		return 0 // open bus: unpopulated on a stock KIM-1
	}
}

// Write implements bus.Bus. Writes into ROM windows (and the aliased
// vector region, and any unmapped address) are no-ops.
func (s *System) Write(addr uint16, v uint8) {
	switch {
	case addr <= ramEnd:
		s.RAM[addr-ramStart] = v
	case addr >= appIOStart && addr <= appIOEnd:
		writeIO(s.App, addr-appIOStart, v)
		if s.DebugIO != nil {
			s.DebugIO("app", addr-appIOStart, v)
		}
	case addr >= kbdIOStart && addr <= kbdIOEnd:
		writeIO(s.Kbd, addr-kbdIOStart, v)
		s.refreshDisplay()
		if s.DebugIO != nil {
			s.DebugIO("kbd", addr-kbdIOStart, v)
		}
	case addr >= appRAMStart && addr <= appRAMEnd:
		s.App.WriteRAM(uint8(addr-appRAMStart), v)
	case addr >= kbdRAMStart && addr <= kbdRAMEnd:
		s.Kbd.WriteRAM(uint8(addr-kbdRAMStart), v)
	}
}

func readIO(r *riot.RIOT, offset uint16) uint8 {
	switch decodeIOOffset(offset) {
	case regPortA:
		return r.PortA.Read()
	case regPortADDR:
		return r.PortA.ReadDDR()
	case regPortB:
		return r.PortB.Read()
	case regPortBDDR:
		return r.PortB.ReadDDR()
	case regTimer1, regTimer64:
		return r.Timer.ReadValue()
	case regTimer8, regTimer1024:
		return r.Timer.ReadInterruptFlag()
	}
	return 0
}

func writeIO(r *riot.RIOT, offset uint16, v uint8) {
	switch decodeIOOffset(offset) {
	case regPortA:
		r.PortA.Write(v)
	case regPortADDR:
		r.PortA.WriteDDR(v)
	case regPortB:
		r.PortB.Write(v)
	case regPortBDDR:
		r.PortB.WriteDDR(v)
	case regTimer1:
		r.Timer.Write(1, false, v)
	case regTimer8:
		r.Timer.Write(8, false, v)
	case regTimer64:
		r.Timer.Write(64, false, v)
	case regTimer1024:
		r.Timer.Write(1024, false, v)
	}
}
