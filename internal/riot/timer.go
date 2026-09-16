package riot

// Timer models the 6530/6532 RIOT's 8-bit interval timer: a free-running
// down-counter with a selectable prescale divisor (1, 8, 64, or 1024 CPU
// cycles per count). Writing the timer latches a new starting value and
// divisor; reading it returns the live count and clears the underflow
// (interrupt) flag as a side effect, matching documented RIOT behavior.
//
// Flag: the exact underflow free-run behavior below (wrapping to $FF and
// continuing to count at divide-by-1) matches the commonly documented
// 6532 datasheet behavior, but should be cross-checked against the
// datasheet if KIM-1 software is found to depend on finer timing details.
type Timer struct {
	value       uint8
	prescale    uint16
	divider     uint16
	irqEnabled  bool
	underflowed bool
}

// Write latches a new count value and prescale divisor, and starts the
// countdown. divider must be one of 1, 8, 64, or 1024.
func (t *Timer) Write(divider uint16, irqEnabled bool, value uint8) {
	t.divider = divider
	t.irqEnabled = irqEnabled
	t.value = value
	t.prescale = divider
	t.underflowed = false
}

// Tick advances the timer by the given number of CPU cycles.
func (t *Timer) Tick(cycles int) {
	remaining := cycles
	for remaining > 0 {
		step := remaining
		if int(t.prescale) < step {
			step = int(t.prescale)
		}
		t.prescale -= uint16(step)
		remaining -= step
		if t.prescale == 0 {
			if t.value == 0 {
				t.underflowed = true
				t.value = 0xFF
				t.prescale = 1 // free-runs at divide-by-1 after underflow
			} else {
				t.value--
				t.prescale = t.divider
			}
		}
	}
}

// ReadValue returns the live countdown value and clears the underflow
// (interrupt) flag, per documented RIOT timer-read semantics.
func (t *Timer) ReadValue() uint8 {
	v := t.value
	t.underflowed = false
	return v
}

// PeekValue returns the live countdown value without side effects, for
// debug/UI display.
func (t *Timer) PeekValue() uint8 { return t.value }

// ReadInterruptFlag returns the underflow flag in bit 7, without
// disturbing the count or clearing the flag.
func (t *Timer) ReadInterruptFlag() uint8 {
	if t.underflowed {
		return 0x80
	}
	return 0
}

// Underflowed reports whether the timer has underflowed since the flag
// was last cleared (used by System to drive CPU.IRQ when IRQEnabled).
func (t *Timer) Underflowed() bool { return t.underflowed }

// IRQEnabled reports whether the timer was configured to assert IRQ on
// underflow.
func (t *Timer) IRQEnabled() bool { return t.irqEnabled }
