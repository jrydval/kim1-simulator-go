package kim1

// Display models the KIM-1's 6-digit multiplexed 7-segment LED display as
// a shadow buffer. The real monitor ROM lights one digit at a time (Port B
// selects it via the 74145 decoder, Port A drives its segments) many times
// per second, relying on persistence of vision; rather than simulating
// that refresh timing, Display just remembers the most recently written
// segment pattern per digit, which converges to the correct steady-state
// image within a few milliseconds of real scan-loop activity. See
// docs/kim1-memory-map.md and system.go's refreshDisplay.
//
// Digits store the raw 7-bit Port A output pattern as last latched by the
// ROM, deliberately not decoded into named segments (a-g) here — the
// exact segment-to-bit assignment is unconfirmed against a schematic, so
// that mapping is isolated to the presentation layer (web UI), which can
// be corrected in one place without touching this model.
type Display struct {
	Digits [6]uint8
}

// NewDisplay returns a Display with all digits blank.
func NewDisplay() *Display {
	return &Display{}
}

// Update latches a new raw segment pattern for the given digit index (0-5).
func (d *Display) Update(digit int, segments uint8) {
	if digit < 0 || digit >= len(d.Digits) {
		return
	}
	d.Digits[digit] = segments & 0x7F
}
