package kim1

// TTY models a teletype on the KIM-1's two 20mA current loops, as wired
// on the schematic (docs/kim-1-schematic.pdf) around the Kbd RIOT
// (6530-002):
//
//   - KBD loop (teletype keyboard -> KIM-1): Q7 converts loop current
//     to a logic level, and U15D (7400) presents it on Port A bit 7 as
//     PA7 = NAND(Q7 collector, NOT PB5). With PB5 low the keyboard line
//     passes straight through (mark = 1, space/start = 0); with PB5 high
//     PA7 is held at idle-mark, i.e. PB5 is a TTY input inhibit (it also
//     gates the cassette input via U26D, not modelled here).
//   - PTR loop (KIM-1 -> teletype printer): U26C (7438, open collector)
//     drives it as NAND(PA7, PB0), so the printer sees mark only while
//     both PA7 and PB0 are high. That makes the echo of typed characters
//     pure hardware: every bit arriving on PA7 is copied onto the printer
//     line as it arrives, whether or not the CPU is even listening. The
//     monitor's GETCH ($1E5A) accordingly never echoes anything itself;
//     OUTCH ($1EA0) bit-bangs PB0 while PA7 idles high.
//
// Framing is 1 start bit + 8 data bits (LSB first) + 1 stop bit, both
// lines idle-high. The monitor has no baud-rate register: on reset it
// measures the width (in CPU cycles) of the very first start bit it sees
// and reuses that as its per-bit delay for both directions from then on.
// A real teletype's bootstrap procedure exploits this by sending RUBOUT
// ($7F, all-1 data bits) first, which produces a start pulse exactly one
// bit wide. CyclesPerBit is the terminal's own baud rate, used both to
// send on the KBD loop and to sample the PTR loop -- like a real
// terminal, it only decodes the KIM-1's output correctly if the monitor
// calibrated to (roughly) the same rate.
type TTY struct {
	CyclesPerBit uint64

	// OnByte, if set, is called with each byte decoded from the PTR
	// loop -- what the teletype's printer would print, echoed keystrokes
	// included. Called synchronously from whatever goroutine is driving
	// System.Step (typically already holding whatever lock guards system
	// state).
	OnByte func(b byte)

	// --- KBD loop: bytes queued here go out one bit at a time. ---
	kbdQueue []kbdByte
	kbdBusy  bool
	kbdByte  byte
	kbdStart uint64

	// --- PTR loop: a sampling UART receiver, like the terminal's own. ---
	ptrState  ptrState
	ptrStart  uint64
	ptrBit    int // next bit to sample: 0 = start, 1-8 = data, 9 = stop
	ptrByte   byte
	ptrFrames int // framing errors seen, for tests/diagnostics
}

// kbdByte is one byte waiting to go out on the KBD loop. A paced byte
// (see Send) waits for the CPU to poll PA7 before starting; an unpaced
// one (see Type) starts as soon as the line is free.
type kbdByte struct {
	b     byte
	paced bool
}

type ptrState int

const (
	ptrIdle ptrState = iota
	ptrReceiving
	ptrWaitMark // after a framing error: wait for the line to go idle
)

// NewTTY returns an idle TTY using the given bit period (in CPU cycles).
func NewTTY(cyclesPerBit uint64) *TTY {
	return &TTY{CyclesPerBit: cyclesPerBit}
}

// Reset clears anything in flight on either loop, as happens on a real
// teletype connection when the KIM-1 is reset mid-byte.
func (t *TTY) Reset() {
	t.kbdQueue = nil
	t.kbdBusy = false
	t.ptrState = ptrIdle
}

// Type queues bytes as keystrokes: each goes out on the KBD loop as soon
// as the line is free, exactly like a real terminal. If the CPU isn't
// sitting in GETCH at that moment (e.g. it's busy printing), the byte is
// lost or garbled -- still echoed by the hardware, though, as long as
// PB5 leaves the input open.
func (t *TTY) Type(data ...byte) {
	for _, b := range data {
		t.kbdQueue = append(t.kbdQueue, kbdByte{b: b})
	}
}

// Send queues bytes paced to the CPU: each one waits on the line until
// the CPU next reads Port A with the TTY input enabled (PB5 low), so
// that bulk text (a paste, the post-reset RUBOUT) isn't lost while the
// program is busy between GETCH calls. Think of it as a terminal with
// perfect flow control; the bytes themselves still travel the wire bit
// by bit and are echoed by hardware like any other.
func (t *TTY) Send(data ...byte) {
	for _, b := range data {
		t.kbdQueue = append(t.kbdQueue, kbdByte{b: b, paced: true})
	}
}

// Pending reports how many bytes are still waiting to go out on the KBD
// loop (not counting one already in flight).
func (t *TTY) Pending() int { return len(t.kbdQueue) }

// KbdLevel returns the KBD loop's level at CPU cycle now (1 = mark/idle,
// 0 = space), starting the next unpaced byte if the line is free.
func (t *TTY) KbdLevel(now uint64) uint8 {
	t.kbdBitAt(now) // retire a finished byte first
	if !t.kbdBusy && len(t.kbdQueue) > 0 && !t.kbdQueue[0].paced {
		t.startKbdByte(now)
	}
	return t.kbdBitAt(now)
}

// CPUPolled tells the TTY the CPU just read PA7 with the input enabled,
// which is what releases the next paced byte (see Send).
func (t *TTY) CPUPolled(now uint64) {
	t.kbdBitAt(now) // retire a finished byte first
	if !t.kbdBusy && len(t.kbdQueue) > 0 {
		t.startKbdByte(now)
	}
}

func (t *TTY) startKbdByte(now uint64) {
	t.kbdByte = t.kbdQueue[0].b
	t.kbdQueue = t.kbdQueue[1:]
	t.kbdStart = now
	t.kbdBusy = true
}

// kbdBitAt returns the in-flight byte's level at now, retiring it once
// its stop bit has passed.
func (t *TTY) kbdBitAt(now uint64) uint8 {
	if !t.kbdBusy {
		return 1
	}
	var elapsed uint64
	if now > t.kbdStart {
		elapsed = (now - t.kbdStart) / t.CyclesPerBit
	}
	switch {
	case elapsed == 0:
		return 0 // start bit
	case elapsed <= 8:
		return (t.kbdByte >> uint(elapsed-1)) & 1
	case elapsed == 9:
		return 1 // stop bit
	default:
		t.kbdBusy = false
		return 1
	}
}

// ObservePtr feeds the PTR loop's current level (1 = mark) at CPU cycle
// now into the receiver. Call it regularly -- System does so after every
// instruction, far finer than one bit period. Like a hardware UART it
// detects the start bit's falling edge, then samples each following bit
// in the middle of its period.
func (t *TTY) ObservePtr(now uint64, level uint8) {
	switch t.ptrState {
	case ptrIdle:
		if level == 0 {
			t.ptrState = ptrReceiving
			t.ptrStart = now
			t.ptrBit = 0
			t.ptrByte = 0
		}
	case ptrWaitMark:
		if level != 0 {
			t.ptrState = ptrIdle
		}
	case ptrReceiving:
		sampleAt := t.ptrStart + uint64(t.ptrBit)*t.CyclesPerBit + t.CyclesPerBit/2
		if now < sampleAt {
			return
		}
		switch {
		case t.ptrBit == 0:
			if level != 0 {
				t.ptrState = ptrIdle // glitch, not a real start bit
				return
			}
		case t.ptrBit <= 8:
			t.ptrByte |= level << uint(t.ptrBit-1)
		default: // stop bit
			if level == 0 {
				t.ptrFrames++
				t.ptrState = ptrWaitMark
				return
			}
			t.ptrState = ptrIdle
			if t.OnByte != nil {
				t.OnByte(t.ptrByte)
			}
			return
		}
		t.ptrBit++
	}
}
