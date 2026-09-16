package webui

import (
	"fmt"

	"6502/internal/bus"
	"6502/internal/cpu"
)

// segmentToHex maps a raw 7-segment pattern (as stored in kim1.Display,
// bits 0-6 = segments a-g) back to its hex digit character. Matches the
// real monitor ROM's own segment table — see docs/kim1-memory-map.md.
var segmentToHex = map[uint8]byte{
	0x3F: '0', 0x06: '1', 0x5B: '2', 0x4F: '3',
	0x66: '4', 0x6D: '5', 0x7D: '6', 0x07: '7',
	0x7F: '8', 0x6F: '9', 0x77: 'A', 0x7C: 'B',
	0x39: 'C', 0x5E: 'D', 0x79: 'E', 0x71: 'F',
}

// decodeDisplayAddress reads the KIM-1 display's 4 address digits (the
// leftmost of the 6 shown — see kim1.Display) back into a 16-bit address,
// the same value a user reading the physical LEDs would see. Returns
// ok=false if any of the 4 digits isn't a currently-recognized segment
// pattern (e.g. blank).
func decodeDisplayAddress(digits [6]uint8) (addr uint16, ok bool) {
	var v uint16
	for i := 0; i < 4; i++ {
		c, found := segmentToHex[digits[i]]
		if !found {
			return 0, false
		}
		var nibble uint16
		if c >= 'A' {
			nibble = uint16(c-'A') + 10
		} else {
			nibble = uint16(c - '0')
		}
		v = v<<4 | nibble
	}
	return v, true
}

// disassembleAtDisplayAddress disassembles the instruction at whatever
// address is currently shown on the KIM-1's display — what the user is
// actually examining via AD/DA/+, which is usually far more useful than
// the CPU's own PC (almost always deep in idle monitor code).
func disassembleAtDisplayAddress(b bus.Bus, digits [6]uint8) string {
	addr, ok := decodeDisplayAddress(digits)
	if !ok {
		return "----:  ?"
	}
	instr, _ := cpu.Disassemble(b, addr)
	return fmt.Sprintf("%04X:  %s", addr, instr)
}
