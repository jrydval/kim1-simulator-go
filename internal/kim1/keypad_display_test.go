package kim1

import "testing"

const (
	kbdPAData = kbdIOStart + 0
	kbdPADDR  = kbdIOStart + 1
	kbdPBData = kbdIOStart + 2
	kbdPBDDR  = kbdIOStart + 3
)

func TestDisplayLatchesSelectedDigit(t *testing.T) {
	s := New()
	s.Write(kbdPADDR, 0x7F)  // Port A all output (segment drive)
	s.Write(kbdPAData, 0x55) // segment pattern, latched once a digit is selected
	s.Write(kbdPBData, 6<<1) // 74145 code 6 -> line 6 -> digit index 2

	if got := s.Display.Digits[2]; got != 0x55 {
		t.Fatalf("Display.Digits[2] = %02X, want 0x55", got)
	}
	for i, d := range s.Display.Digits {
		if i != 2 && d != 0 {
			t.Fatalf("Display.Digits[%d] = %02X, want 0 (unaffected)", i, d)
		}
	}
}

func TestDisplayScanSequenceAcrossAllDigits(t *testing.T) {
	s := New()
	s.Write(kbdPADDR, 0x7F)
	// A real scan loop selects the digit *before* loading its segment
	// pattern, so any single-write "flicker" onto the previously-selected
	// digit is immediately corrected by that digit's own next PA write.
	// Writing PA before PB (segments-then-select) would instead leave a
	// stale pattern latched on the previously-selected digit — accurately
	// reflecting how the real hardware's continuously-active mux works,
	// not a shadow-buffer bug.
	for digit := 0; digit < 6; digit++ {
		pattern := uint8(0x10 + digit)
		s.Write(kbdPBData, uint8(digit+4)<<1) // line 4-9
		s.Write(kbdPAData, pattern)
	}
	for digit := 0; digit < 6; digit++ {
		want := uint8(0x10 + digit)
		if got := s.Display.Digits[digit]; got != want {
			t.Fatalf("Display.Digits[%d] = %02X, want %02X", digit, got, want)
		}
	}
}

func TestDisplayIgnoresTransientBlankBeforeNextSelect(t *testing.T) {
	// Regression test for the real KIM-1 monitor ROM's actual scan
	// pattern: select digit -> write real segments -> hold -> write an
	// all-zero pattern for a handful of cycles immediately before
	// selecting the next digit. On real hardware that blank pulse is far
	// too brief to be visible; a shadow buffer that took it at face
	// value showed a flickering, mostly-blank display instead of the
	// intended steady image (observed when bringing up real ROMs).
	s := New()
	s.Write(kbdPADDR, 0x7F)
	s.Write(kbdPBData, 5<<1) // select digit index 1
	s.Write(kbdPAData, 0x3F) // real segment pattern
	s.Write(kbdPAData, 0x00) // transitional blank, still on digit 1 — must NOT clear it

	if got := s.Display.Digits[1]; got != 0x3F {
		t.Fatalf("Display.Digits[1] = %02X, want 0x3F (transient blank before deselect must be ignored)", got)
	}

	s.Write(kbdPBData, 6<<1) // move on to digit index 2 — digit 1 must stay as it was
	if got := s.Display.Digits[1]; got != 0x3F {
		t.Fatalf("Display.Digits[1] = %02X after moving to next digit, want 0x3F still", got)
	}
}

func TestDisplayIgnoresNonDigitMuxLines(t *testing.T) {
	s := New()
	s.Write(kbdPADDR, 0x7F)
	s.Write(kbdPAData, 0x7F)
	s.Write(kbdPBData, 3<<1) // 74145 line 3: unused (not a row, not a digit)
	for i, d := range s.Display.Digits {
		if d != 0 {
			t.Fatalf("Display.Digits[%d] = %02X, want 0 (line 3 is unused)", i, d)
		}
	}
}

func TestKeypadRowScanReadsPressedKey(t *testing.T) {
	s := New()
	s.Keypad.SetPressed(1, 3, true) // row 1, column 3 held

	s.Write(kbdPADDR, 0x00)  // Port A all input (column read mode)
	s.Write(kbdPBData, 1<<1) // 74145 code 1 -> row 1

	// Bit 7 reads back 1 (idle/mark) alongside the column bits: it's
	// always the TTY RX line (see TTY's doc comment), regardless of
	// what the keypad row-select is doing.
	got := s.Read(kbdPAData)
	want := uint8(0xFF &^ (1 << 3))
	if got != want {
		t.Fatalf("Read(PA) = %02X, want %02X (column 3 pulled low, bit 7 = idle TTY RX)", got, want)
	}
}

func TestKeypadRowScanOtherRowUnaffected(t *testing.T) {
	s := New()
	s.Keypad.SetPressed(1, 3, true)

	s.Write(kbdPADDR, 0x00)
	s.Write(kbdPBData, 0<<1) // select row 0, where nothing is pressed

	if got := s.Read(kbdPAData); got != 0xFF {
		t.Fatalf("Read(PA) on unpressed row = %02X, want 0xFF", got)
	}
}

func TestKeypadReleasedKeyStopsReading(t *testing.T) {
	s := New()
	s.Keypad.SetPressed(0, 0, true)
	s.Write(kbdPADDR, 0x00)
	s.Write(kbdPBData, 0)
	if got := s.Read(kbdPAData); got == 0xFF {
		t.Fatalf("expected column 0 pulled low while pressed")
	}

	s.Keypad.SetPressed(0, 0, false)
	if got := s.Read(kbdPAData); got != 0xFF {
		t.Fatalf("Read(PA) after release = %02X, want 0xFF", got)
	}
}
