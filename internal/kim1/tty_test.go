package kim1

import (
	"strings"
	"testing"
)

func TestTTYKbdIdleIsMark(t *testing.T) {
	tty := NewTTY(100)
	if got := tty.KbdLevel(0); got != 1 {
		t.Fatalf("KbdLevel while idle = %d, want 1 (mark)", got)
	}
}

func TestTTYTypeFraming(t *testing.T) {
	tty := NewTTY(100)
	tty.Type(0xA5) // 10100101, LSB first: 1,0,1,0,0,1,0,1

	if got := tty.KbdLevel(0); got != 0 {
		t.Fatalf("start bit = %d, want 0", got)
	}
	// Sampling at the middle of each bit period should reproduce: start
	// (0), then the 8 data bits LSB-first, then stop (1).
	want := []uint8{0, 1, 0, 1, 0, 0, 1, 0, 1, 1}
	for i, w := range want {
		now := uint64(i)*100 + 50
		if got := tty.KbdLevel(now); got != w {
			t.Fatalf("bit %d: KbdLevel(%d) = %d, want %d", i, now, got, w)
		}
	}
	if got := tty.KbdLevel(1050); got != 1 {
		t.Fatalf("KbdLevel after byte = %d, want 1 (idle)", got)
	}
}

func TestTTYTypedBytesGoBackToBack(t *testing.T) {
	tty := NewTTY(100)
	tty.Type(0x01, 0x02)

	tty.KbdLevel(0)
	if got := tty.KbdLevel(150); got != 1 {
		t.Fatalf("first byte data bit 0 = %d, want 1 (0x01's LSB)", got)
	}
	// The second byte starts as soon as the first one's stop bit ends,
	// whether or not anybody is reading.
	if got := tty.KbdLevel(1050); got != 0 {
		t.Fatalf("second byte start bit = %d, want 0", got)
	}
}

func TestTTYSendWaitsForCPUPoll(t *testing.T) {
	tty := NewTTY(100)
	tty.Send(0x00)

	if got := tty.KbdLevel(5000); got != 1 {
		t.Fatalf("paced byte started without a CPU poll: level = %d", got)
	}
	tty.CPUPolled(6000)
	if got := tty.KbdLevel(6050); got != 0 {
		t.Fatalf("paced byte start bit after poll = %d, want 0", got)
	}
}

// drivePtr puts b on a simulated PTR line with the given bit period,
// sampling every step cycles like System.Step does.
func drivePtr(tty *TTY, start uint64, bitCycles uint64, b byte, step uint64) uint64 {
	level := func(now uint64) uint8 {
		if now < start {
			return 1
		}
		switch bit := (now - start) / bitCycles; {
		case bit == 0:
			return 0
		case bit <= 8:
			return (b >> uint(bit-1)) & 1
		default:
			return 1
		}
	}
	end := start + 11*bitCycles
	for now := start; now < end; now += step {
		tty.ObservePtr(now, level(now))
	}
	return end
}

func TestTTYObservePtrDecodesBytes(t *testing.T) {
	tty := NewTTY(100)
	var got []byte
	tty.OnByte = func(b byte) { got = append(got, b) }

	now := drivePtr(tty, 0, 100, 'K', 3)
	// A sender running 4% slow is still decoded, as by a real UART.
	drivePtr(tty, now, 104, 'I', 3)

	if string(got) != "KI" {
		t.Fatalf("decoded = %q, want %q", got, "KI")
	}
}

func TestTTYObservePtrFramingError(t *testing.T) {
	tty := NewTTY(100)
	called := false
	tty.OnByte = func(b byte) { called = true }

	for now := uint64(0); now < 2000; now += 5 {
		tty.ObservePtr(now, 0) // line stuck at space: a "break"
	}
	if called {
		t.Fatal("OnByte fired for a break (no stop bit)")
	}
	if tty.ptrFrames != 1 {
		t.Fatalf("framing errors = %d, want 1", tty.ptrFrames)
	}
}

// System-level wiring: no ROM needed since these only exercise the
// TTYSelect/Reset/Port plumbing directly.

func TestSystemTTYSelectAffectsIdlePortABit0(t *testing.T) {
	s := New()
	s.Write(kbdPADDR, 0x00)
	s.Write(kbdPBData, 3<<1) // 74145 line 3: not a keyboard row

	if got := s.Read(kbdPAData); got&1 != 1 {
		t.Fatalf("Port A bit 0 = %d with TTYSelect off, want 1 (keyboard mode)", got&1)
	}

	s.TTYSelect = true
	if got := s.Read(kbdPAData); got&1 != 0 {
		t.Fatalf("Port A bit 0 = %d with TTYSelect on, want 0 (TTY mode)", got&1)
	}
}

func TestSystemPB5InhibitsTTYInput(t *testing.T) {
	s := New()
	s.Write(kbdPADDR, 0x00)
	s.Write(kbdPBDDR, 0x3F)
	s.Write(kbdPBData, 0x27) // PB5 high: input inhibited
	s.TTY.Type(0x00)

	if got := s.Read(kbdPAData) >> 7; got != 1 {
		t.Fatalf("PA7 with PB5 high = %d, want 1 (inhibited)", got)
	}
	s.Write(kbdPBData, 0x07) // PB5 low: input enabled
	if got := s.Read(kbdPAData) >> 7; got != 0 {
		t.Fatalf("PA7 with PB5 low = %d, want 0 (start bit passes through)", got)
	}
}

func TestSystemPTRIsPA7AndPB0(t *testing.T) {
	s := New()
	s.Write(kbdPBDDR, 0x3F)
	s.Write(kbdPBData, 0x07)
	if got := s.ttyPTR(); got != 1 {
		t.Fatalf("PTR idle = %d, want 1", got)
	}
	s.Write(kbdPBData, 0x06) // PB0 low: CPU transmitting a space
	if got := s.ttyPTR(); got != 0 {
		t.Fatalf("PTR with PB0 low = %d, want 0", got)
	}
	s.Write(kbdPBData, 0x07)
	s.TTY.Type(0x00)
	if got := s.ttyPTR(); got != 0 {
		t.Fatalf("PTR during incoming start bit = %d, want 0 (hardware echo)", got)
	}
}

func TestSystemResetQueuesRuboutWhenTTYSelected(t *testing.T) {
	s := New()
	s.TTYSelect = true
	s.Reset()

	if len(s.TTY.kbdQueue) != 1 || s.TTY.kbdQueue[0] != (kbdByte{b: 0x7F, paced: true}) {
		t.Fatalf("kbdQueue after Reset = %v, want [paced 7F] (RUBOUT, for auto-baud calibration)", s.TTY.kbdQueue)
	}
}

func TestSystemResetDoesNotQueueRuboutWithoutTTYSelect(t *testing.T) {
	s := New()
	s.Reset()

	if len(s.TTY.kbdQueue) != 0 {
		t.Fatalf("kbdQueue after Reset = %v, want empty (TTYSelect off)", s.TTY.kbdQueue)
	}
}

// --- Against the real monitor ROM (skipped when it isn't available). ---

// newROMSystem returns a System running the real monitor ROM, or skips
// the test if the (copyrighted, not committed) ROM images are missing.
func newROMSystem(t *testing.T) *System {
	t.Helper()
	s := New()
	for _, load := range []struct {
		fn   func(string) error
		path string
	}{
		{s.LoadAppROM, "../../testdata/6530-003.bin"},
		{s.LoadKbdROM, "../../testdata/6530-002.bin"},
	} {
		if err := load.fn(load.path); err != nil {
			t.Skipf("skipping: %v", err)
		}
	}
	return s
}

// runCycles steps s for at least n cycles.
func runCycles(s *System, n uint64) {
	for end := s.CPU.Cycles + n; s.CPU.Cycles < end; {
		s.Step()
	}
}

// printer collects everything decoded from the PTR loop.
func printer(s *System) *strings.Builder {
	var out strings.Builder
	s.TTY.OnByte = func(b byte) { out.WriteByte(b) }
	return &out
}

// charCycles is one 10-bit character time at the default baud rate.
const charCycles = 10 * defaultTTYCyclesPerBit

func TestROMTTYBootsAndEchoesTyping(t *testing.T) {
	s := newROMSystem(t)
	out := printer(s)
	s.TTYSelect = true
	s.Reset()
	runCycles(s, 100*charCycles)
	if !strings.Contains(out.String(), "KIM") {
		t.Fatalf("after reset the printer shows %q, want the KIM prompt", out.String())
	}

	// Type at human speed: one key every ~5 character times. Each key is
	// echoed purely by hardware (GETCH never transmits), and the space
	// makes the monitor open $0200 and print its address and contents.
	out.Reset()
	s.RAM[0x0200] = 0x5A
	for _, c := range []byte("0200 ") {
		s.TTY.Type(c)
		runCycles(s, 5*charCycles)
	}
	runCycles(s, 30*charCycles)
	if got := out.String(); !strings.HasPrefix(got, "0200 ") || !strings.Contains(got, "0200 5A") {
		t.Fatalf("printer shows %q, want the typed echo followed by \"0200 5A\"", got)
	}
}

func TestROMTTYPastedTextIsPaced(t *testing.T) {
	s := newROMSystem(t)
	printer(s)
	s.TTYSelect = true
	s.Reset()
	runCycles(s, 100*charCycles)

	// Queued all at once: without pacing, the monitor would still be
	// printing "0200 xx " when the data bytes arrived and lose them.
	s.TTY.Send([]byte("0200 A9.01.EA.")...)
	for i := 0; i < 200 && s.TTY.Pending() > 0; i++ {
		runCycles(s, 10*charCycles)
	}
	runCycles(s, 30*charCycles)
	if got := s.RAM[0x0200:0x0203]; got[0] != 0xA9 || got[1] != 0x01 || got[2] != 0xEA {
		t.Fatalf("RAM $0200-$0202 = % X, want A9 01 EA", got)
	}
}

func TestROMKeypadModePrintsNothing(t *testing.T) {
	s := newROMSystem(t)
	out := printer(s)
	s.Reset()
	runCycles(s, 2_000_000) // two seconds of display multiplexing
	if out.Len() != 0 || s.TTY.ptrFrames != 0 {
		t.Fatalf("keypad mode put %q (%d framing errors) on the printer loop", out.String(), s.TTY.ptrFrames)
	}
}
