package kim1

// decode74145 models the external 74145 BCD-to-decimal decoder used to
// multiplex the KIM-1 keypad/display: given a 4-bit BCD code, it returns
// which single output line (0-9) is active, or -1 for the unused/invalid
// codes 10-15. Per the KIM-1 hardware overview, outputs 0-2 select
// keyboard rows and outputs 4-9 select one of the 6 display digits
// (output 3 is unused).
func decode74145(code uint8) int {
	if code > 9 {
		return -1
	}
	return int(code)
}

// kbdMuxLine reads the current 74145 select code from the Kbd RIOT's Port
// B bits 1-4 and decodes it.
func (s *System) kbdMuxLine() int {
	code := (s.Kbd.PortB.OutputData() >> 1) & 0x0F
	return decode74145(code)
}

// refreshDisplay latches Port A's current output into the Display's
// shadow buffer if the 74145 is currently selecting a display digit
// (lines 4-9). Called after every write to the Kbd RIOT's I/O registers.
func (s *System) refreshDisplay() {
	line := s.kbdMuxLine()
	if line < 4 || line > 9 {
		return
	}
	digit := line - 4
	segments := s.Kbd.PortA.OutputData() & s.Kbd.PortA.ReadDDR()
	s.Display.Update(digit, segments, s.CPU.Cycles)
}

// keypadColumnInput supplies Port A's input-mode bits. Bit 7 is always
// the TTY's RX line (idle-high), sampled by GETCH regardless of what the
// keypad/display multiplexer is doing at that instant -- confirmed by
// disassembling the real ROM, where GETCH's "BIT SAD" doesn't check the
// 74145 select line at all. Bits 0-6 come from whichever keyboard row
// (lines 0-2) the 74145 currently selects; outside of an active row
// select, bit 0 instead reflects TTYSelect (0 = TTY mode, matching the
// monitor's own reset-time and main-loop "BIT SAD" checks of that bit),
// with bits 1-6 idle-high.
func (s *System) keypadColumnInput() uint8 {
	rx := s.TTY.RXBit(s.CPU.Cycles) << 7

	line := s.kbdMuxLine()
	if line < 0 || line > 2 {
		if s.TTYSelect {
			return rx | 0x7E
		}
		return rx | 0x7F
	}
	return rx | s.Keypad.ColumnBits(line)
}
