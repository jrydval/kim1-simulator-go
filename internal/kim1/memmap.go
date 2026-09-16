package kim1

// KIM-1 address map, cross-checked against the published KIM-1 User's
// Manual memory map and the register-address constants used by
// reconstructed/disassembled monitor ROM source listings (PAD/PADD/PBD/
// PBDD at $1700, SAD/../SBD/PBDD and CLK1T/CLK8T/CLK64T/CLKKT at $1740).
//
// Two MOS 6530 RIOT chips are present:
//   - "App" (silkscreened 6530-003): the 15 I/O pins on this chip are
//     brought out to the KIM-1's application connector for user projects.
//   - "Kbd" (silkscreened 6530-002): dedicated to the on-board hex
//     keypad, 7-segment display, TTY interface, and cassette audio
//     interface.
//
// Each RIOT decodes only 3 address lines (A0-A2) within its own 64-byte
// I/O window, giving 8 registers (Port A data/DDR, Port B data/DDR, and
// the 4 timer-prescale write addresses) mirrored 8 times across the
// window — the standard, widely-documented 6530/6532 RIOT I/O decode
// pattern. See internal/riot for the register semantics.
const (
	ramStart uint16 = 0x0000
	ramEnd   uint16 = 0x03FF // 1KB on-board system RAM

	appIOStart  uint16 = 0x1700 // App RIOT (6530-003) I/O + timer registers
	appIOEnd    uint16 = 0x173F
	kbdIOStart  uint16 = 0x1740 // Kbd RIOT (6530-002) I/O + timer registers
	kbdIOEnd    uint16 = 0x177F
	appRAMStart uint16 = 0x1780 // App RIOT's internal 64-byte RAM
	appRAMEnd   uint16 = 0x17BF
	kbdRAMStart uint16 = 0x17C0 // Kbd RIOT's internal 64-byte RAM
	kbdRAMEnd   uint16 = 0x17FF // holds the NMIV/RSTV/IRQV "soft vectors" at $17FA/$17FC/$17FE
	appROMStart uint16 = 0x1800 // App RIOT's 1KB mask ROM
	appROMEnd   uint16 = 0x1BFF
	kbdROMStart uint16 = 0x1C00 // Kbd RIOT's 1KB mask ROM
	kbdROMEnd   uint16 = 0x1FFF // holds the hardware vector entry points at $1FFA/$1FFC/$1FFE

	// hwVectorStart is where the 6502's own fixed vectors ($FFFA-$FFFF)
	// live. On real hardware, incomplete address decoding of the ROM
	// chip-select aliases this region onto the top of the Kbd RIOT's ROM
	// ($1FFA-$1FFF) — without this alias RESET/IRQ/NMI could never reach
	// the monitor ROM at all. The monitor's fixed ROM vector handlers
	// then do an indirect jump through the RAM "soft vectors" above,
	// which is how KIM-1 software hooks interrupts without touching ROM.
	hwVectorStart uint16 = 0xFFFA
)

// ioRegister identifies one of the 8 mirrored registers within a RIOT's
// I/O window, selected by the low 3 address bits.
type ioRegister uint16

const (
	regPortA ioRegister = iota
	regPortADDR
	regPortB
	regPortBDDR
	regTimer1
	regTimer8
	regTimer64
	regTimer1024
)

func decodeIOOffset(offset uint16) ioRegister {
	return ioRegister(offset & 0x07)
}
