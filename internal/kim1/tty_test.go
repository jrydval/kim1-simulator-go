package kim1

import "testing"

func TestTTYRXBitIdleIsMark(t *testing.T) {
	tty := NewTTY(100)
	if got := tty.RXBit(0); got != 1 {
		t.Fatalf("RXBit while idle = %d, want 1 (mark)", got)
	}
}

func TestTTYRXBitFraming(t *testing.T) {
	tty := NewTTY(100)
	tty.Send(0xA5) // 10100101, LSB first: 1,0,1,0,0,1,0,1

	// Sampling at the middle of each bit period should reproduce: start
	// (0), then the 8 data bits LSB-first, then stop (1).
	want := []uint8{0, 1, 0, 1, 0, 0, 1, 0, 1, 1}
	for i, w := range want {
		now := uint64(i)*100 + 50
		if got := tty.RXBit(now); got != w {
			t.Fatalf("bit %d: RXBit(%d) = %d, want %d", i, now, got, w)
		}
	}
	// Back to idle afterwards.
	if got := tty.RXBit(1050); got != 1 {
		t.Fatalf("RXBit after byte = %d, want 1 (idle)", got)
	}
}

func TestTTYRXBitQueueOrder(t *testing.T) {
	tty := NewTTY(100)
	tty.Send(0x01, 0x02)

	// First byte's start bit at t=0, data bit 0 (LSB, =1) at t=100.
	if got := tty.RXBit(50); got != 0 {
		t.Fatalf("first byte start bit = %d, want 0", got)
	}
	if got := tty.RXBit(150); got != 1 {
		t.Fatalf("first byte data bit 0 = %d, want 1 (0x01's LSB)", got)
	}
	// Skip ahead past the first byte's 10 bit periods (1000 cycles) into
	// the second byte's start bit.
	if got := tty.RXBit(1050); got != 0 {
		t.Fatalf("second byte start bit = %d, want 0", got)
	}
}

func TestTTYObserveTxWriteDecodesByte(t *testing.T) {
	tty := NewTTY(100)
	var got []byte
	tty.OnByte = func(b byte) { got = append(got, b) }

	send := func(b byte) {
		tty.ObserveTxWrite(0) // start bit
		for i := 0; i < 8; i++ {
			tty.ObserveTxWrite((b >> uint(i)) & 1) // data bits, LSB first
		}
		tty.ObserveTxWrite(1) // stop bit
	}
	send(0x4B) // 'K'
	send(0x49) // 'I'

	if string(got) != "KI" {
		t.Fatalf("decoded = %q, want %q", got, "KI")
	}
}

func TestTTYObserveTxWriteIgnoresIdleLevel(t *testing.T) {
	tty := NewTTY(100)
	called := false
	tty.OnByte = func(b byte) { called = true }

	for i := 0; i < 20; i++ {
		tty.ObserveTxWrite(1) // idle writes, e.g. from display-mux traffic
	}
	if called {
		t.Fatal("OnByte fired from idle-level writes")
	}
}

// System-level wiring: no ROM needed since these only exercise the
// TTYSelect/Reset/Port A plumbing directly.

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

func TestSystemResetQueuesRuboutWhenTTYSelected(t *testing.T) {
	s := New()
	s.TTYSelect = true
	s.Reset()

	if len(s.TTY.feedQueue) != 1 || s.TTY.feedQueue[0] != 0x7F {
		t.Fatalf("feedQueue after Reset = %v, want [7F] (RUBOUT, for auto-baud calibration)", s.TTY.feedQueue)
	}
}

func TestSystemResetDoesNotQueueRuboutWithoutTTYSelect(t *testing.T) {
	s := New()
	s.Reset()

	if len(s.TTY.feedQueue) != 0 {
		t.Fatalf("feedQueue after Reset = %v, want empty (TTYSelect off)", s.TTY.feedQueue)
	}
}
