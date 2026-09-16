package kim1

// Keypad models the KIM-1's hex keypad matrix. The Kbd RIOT's monitor ROM
// scans it by writing a row-select code to Port B (decoded through an
// external 74145 BCD-to-decimal decoder — see decoder.go) and then
// reading Port A, whose bits 0-6 read back active-low for any held key in
// the currently-selected row.
//
// Flag: the row-scan mechanism (74145 decode, active-low column read on
// Port A) is well documented; the exact physical key-to-(row,column)
// assignment below is a best-effort placeholder and should be
// cross-checked against a KIM-1 keypad schematic/diagram before relying
// on it to match real physical key legends.
type Keypad struct {
	pressed [3][7]bool // [row][column], matching the 3 row-select lines and 7 usable Port A bits
}

// NewKeypad returns a Keypad with no keys held.
func NewKeypad() *Keypad {
	return &Keypad{}
}

// SetPressed marks the key at (row, column) pressed (down=true) or
// released (down=false). Out-of-range coordinates are ignored.
func (k *Keypad) SetPressed(row, column int, down bool) {
	if row < 0 || row >= len(k.pressed) || column < 0 || column >= len(k.pressed[0]) {
		return
	}
	k.pressed[row][column] = down
}

// ColumnBits returns the active-low Port A column read for the given
// scanned row: a 0 bit means the corresponding key is currently held, a 1
// bit means it is idle. Bit 7 is unused (only 7 columns, PA0-PA6).
func (k *Keypad) ColumnBits(row int) uint8 {
	if row < 0 || row >= len(k.pressed) {
		return 0x7F
	}
	bits := uint8(0x7F)
	for col, down := range k.pressed[row] {
		if down {
			bits &^= 1 << uint(col)
		}
	}
	return bits
}
