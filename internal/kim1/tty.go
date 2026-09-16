package kim1

// TTY models a bit-banged serial teletype connection to the Kbd RIOT
// (6530-002). Confirmed by disassembling the real monitor ROM's GETCH
// ($1E5A) and OUTCH ($1EA0) routines: RX arrives on Port A bit 7, TX
// leaves on Port B bit 0, both idle-high (mark = 1, space/start = 0),
// framed as 1 start bit + 8 data bits (LSB first) + 1 stop bit -- the
// monitor doesn't actually check/generate real parity despite what some
// documentation calls the 8th bit, so this models a plain 8-bit UART.
//
// The monitor has no baud-rate register: on reset it measures the width
// (in CPU cycles) of the very first start bit it sees and reuses that
// measurement as its per-bit delay for both directions from then on. A
// real teletype's bootstrap procedure exploits this by sending RUBOUT
// ($7F, all-1 data bits) first, which produces a start pulse exactly one
// bit wide since every other bit in that byte is idle-level too -- see
// Send's doc comment. TTY.CyclesPerBit fixes the baud rate this emulator
// presents to the ROM; as long as the first byte sent after a reset is a
// RUBOUT, the ROM will correctly calibrate to it.
type TTY struct {
	CyclesPerBit uint64

	// OnByte, if set, is called with each byte the CPU transmits via
	// OUTCH, decoded from Port B bit 0. Called synchronously from
	// whatever goroutine is driving System.Step (typically already
	// holding whatever lock guards system state).
	OnByte func(b byte)

	// --- CPU-facing output: bytes queued here are fed to the CPU's
	// GETCH one bit at a time via RXBit, timed by CyclesPerBit. ---
	feedQueue []byte
	feedByte  byte
	feedBit   int // -1 = idle/between bytes, 0 = start, 1-8 = data, 9 = stop
	feedStart uint64

	// --- CPU-facing input: reconstructs whatever the CPU's OUTCH is
	// transmitting, one Port B bit 0 write at a time (the ROM performs
	// exactly one write per bit period, so no timing is needed here --
	// just counting writes). ---
	capState capState
	capByte  byte
	capCount int
}

type capState int

const (
	capIdle capState = iota
	capCollecting
	capStop
)

// NewTTY returns a TTY with an empty send queue, presenting the given
// bit period (in CPU cycles) to the ROM's auto-baud calibration.
func NewTTY(cyclesPerBit uint64) *TTY {
	return &TTY{CyclesPerBit: cyclesPerBit, feedBit: -1}
}

// Reset clears any in-flight transmission or capture state, as happens
// on a real teletype connection when the KIM-1 is reset mid-byte.
func (t *TTY) Reset() {
	t.feedQueue = nil
	t.feedBit = -1
	t.capState = capIdle
}

// Send queues bytes to be fed to the CPU (as if typed on a teletype),
// appended after anything already queued. The very first byte sent after
// a reset (with TTYSelect on) must be $7F (RUBOUT) so the monitor's
// auto-baud calibration measures a clean one-bit-wide start pulse --
// System.Write's "reset" handling does this automatically.
func (t *TTY) Send(data ...byte) {
	t.feedQueue = append(t.feedQueue, data...)
}

// RXBit returns Port A bit 7's current level (1 = idle/mark, 0 = space)
// for the given absolute CPU cycle count. Called from the Kbd RIOT's
// Port A InputFunc on every CPU read of that port.
func (t *TTY) RXBit(now uint64) uint8 {
	for i := 0; i < 2; i++ { // at most: finish a byte, then start the next
		if t.feedBit == -1 {
			if len(t.feedQueue) == 0 {
				return 1
			}
			t.feedByte = t.feedQueue[0]
			t.feedQueue = t.feedQueue[1:]
			t.feedStart = now
			t.feedBit = 0
		}

		elapsed := (now - t.feedStart) / t.CyclesPerBit
		switch {
		case elapsed == 0:
			return 0 // start bit
		case elapsed <= 8:
			return (t.feedByte >> uint(elapsed-1)) & 1
		case elapsed == 9:
			return 1 // stop bit
		default:
			t.feedBit = -1 // this byte is done; loop around for the next one
		}
	}
	return 1
}

// ObserveTxWrite feeds one Port B bit-0 sample into the transmit
// decoder. Call on every write to the Kbd RIOT's Port B data register;
// the ROM's OUTCH writes it exactly once per bit period (start, 8 data
// bits LSB-first, stop), so byte framing falls out of write order alone.
func (t *TTY) ObserveTxWrite(bit uint8) {
	switch t.capState {
	case capIdle:
		if bit == 0 {
			t.capState = capCollecting
			t.capByte = 0
			t.capCount = 0
		}
	case capCollecting:
		if bit != 0 {
			t.capByte |= 1 << uint(t.capCount)
		}
		t.capCount++
		if t.capCount == 8 {
			t.capState = capStop
		}
	case capStop:
		t.capState = capIdle
		if t.OnByte != nil {
			t.OnByte(t.capByte)
		}
	}
}
