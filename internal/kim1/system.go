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
	TTY     *TTY

	// TTYSelect mirrors the physical TTY/keyboard mode jumper the
	// monitor reads as Port A bit 0 outside of active keypad row-scans:
	// with it on, the monitor skips keypad/display mode entirely at
	// reset and boots into TTY command mode instead (see the real ROM's
	// $1C2F/$1C4F "BIT SAD" checks). Must be set before RESET, since
	// it's only sampled there and in the main command-mode dispatch.
	TTYSelect bool

	// SST mirrors the KIM-1's physical Single-Step slide switch. When on,
	// each instruction fetched from outside ROM (i.e. the user's own
	// program, not the monitor) raises an NMI immediately after it
	// executes — mirroring the real hardware, which watches the 6502's
	// SYNC line to fire NMI on every opcode fetch, with the upper ROM
	// address range masked out so the monitor's own NMI handler (and the
	// rest of the monitor) never single-steps itself. Requires the NMI
	// vector at $17FA/$17FB to be set up by the user first, same as ST.
	SST bool

	// pcHist is a ring buffer of the addresses of the last PCHistoryLen
	// instructions executed (opcode addresses, not interrupt responses),
	// for debug views; see RecentPCs.
	pcHist  [PCHistoryLen]uint16
	pcHead  int
	pcCount int

	// AppSwitchA/AppSwitchB are the input levels presented to the App
	// RIOT's Port A/B pins that are currently configured as inputs (DDR
	// bit 0) -- standing in for the toggle switches a real KIM-1 owner
	// might wire to the application connector, since nothing else drives
	// those pins. A pin currently configured as an output instead reads
	// back whatever the CPU itself last drove, same as real hardware;
	// see riot.Port.Read. Set via SetAppSwitch.
	AppSwitchA uint8
	AppSwitchB uint8

	// DebugIO, if set, is called for every write to either RIOT's I/O
	// register window (post address-decode), for diagnosing real ROM
	// interoperability issues.
	DebugIO func(chip string, offset uint16, v uint8)
}

// PCHistoryLen is how many recently executed instruction addresses
// System remembers.
const PCHistoryLen = 10

// defaultTTYCyclesPerBit is the bit period (in emulated 1MHz CPU cycles)
// TTY presents to the monitor ROM's auto-baud calibration, chosen for a
// usable web-terminal typing speed (300 baud) rather than a real
// teletype's historical ~110 baud.
const defaultTTYCyclesPerBit = 1_000_000 / 300

// New returns a System with empty RAM/ROM. Load ROM images with
// LoadAppROM/LoadKbdROM before calling Reset.
func New() *System {
	s := &System{
		App:     riot.New(),
		Kbd:     riot.New(),
		Keypad:  NewKeypad(),
		Display: NewDisplay(),
		TTY:     NewTTY(defaultTTYCyclesPerBit),
	}
	s.Kbd.PortA.InputFunc = s.keypadColumnInput
	s.App.PortA.InputFunc = func() uint8 { return s.AppSwitchA }
	s.App.PortB.InputFunc = func() uint8 { return s.AppSwitchB }
	s.CPU = cpu.New(s)
	return s
}

// SetAppSwitch sets or clears bit (0-7) of the App RIOT's Port A/B
// switch bank. Only affects the pin's read value while that bit is
// currently configured as an input (DDR 0); a bit configured as an
// output ignores it, same as wiring a switch to a pin the CPU is itself
// driving would on real hardware.
func (s *System) SetAppSwitch(port byte, bit int, on bool) {
	if bit < 0 || bit > 7 {
		return
	}
	var target *uint8
	switch port {
	case 'A':
		target = &s.AppSwitchA
	case 'B':
		target = &s.AppSwitchB
	default:
		return
	}
	if on {
		*target |= 1 << uint(bit)
	} else {
		*target &^= 1 << uint(bit)
	}
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
// hardware reset vector ($FFFC/$FFFD -> $1FFC/$1FFD in Kbd ROM). If
// TTYSelect is on, it also queues the RUBOUT ($7F) byte a real teletype
// sends to trigger the monitor's post-reset auto-baud calibration (see
// TTY's doc comment) -- without it, GETCH/OUTCH would use whatever
// leftover (or zero) delay was in RAM and run far too fast to decode.
func (s *System) Reset() {
	s.CPU.Reset()
	s.pcCount = 0
	s.TTY.Reset()
	if s.TTYSelect {
		s.TTY.Send(0x7F)
	}
}

// Step executes one CPU instruction and ticks both RIOT timers by the
// number of cycles it consumed.
func (s *System) Step() int {
	startPC := s.CPU.PC
	opcode := s.Read(startPC)

	cycles := s.CPU.Step()
	s.App.TickTimer(cycles)
	s.Kbd.TickTimer(cycles)

	if !s.CPU.WasInterrupt() {
		s.pcHist[s.pcHead] = startPC
		s.pcHead = (s.pcHead + 1) % PCHistoryLen
		if s.pcCount < PCHistoryLen {
			s.pcCount++
		}
	}

	if s.SST && startPC < appROMStart && opcode != 0x00 && !s.CPU.WasInterrupt() {
		// Just executed a normal instruction fetched from outside ROM
		// (not BRK, which already enters the interrupt handler on its
		// own, and not an interrupt response already in progress) —
		// raise NMI so the monitor breaks in after exactly this one
		// instruction, matching the SYNC-line-driven hardware mechanism.
		s.CPU.NMI()
	}
	return cycles
}

// RecentPCs returns the addresses of the most recently executed
// instructions, newest first (at most PCHistoryLen). A loop shows up as
// repeated addresses, since this records every executed instruction.
func (s *System) RecentPCs() []uint16 {
	out := make([]uint16, s.pcCount)
	for i := range out {
		out[i] = s.pcHist[(s.pcHead-1-i+2*PCHistoryLen)%PCHistoryLen]
	}
	return out
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

// Peek reads addr without any side effects, for debug views. ok is false
// for addresses whose read has no meaningful side-effect-free value: the
// RIOT I/O register windows (reading a timer register clears its flag)
// and unpopulated space.
func (s *System) Peek(addr uint16) (v uint8, ok bool) {
	switch {
	case addr <= ramEnd,
		addr >= appRAMStart && addr <= kbdROMEnd,
		addr >= hwVectorStart:
		return s.Read(addr), true
	default:
		return 0, false
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
		if decodeIOOffset(addr-kbdIOStart) == regPortB {
			s.TTY.ObserveTxWrite(s.Kbd.PortB.OutputData() & 1)
		}
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
