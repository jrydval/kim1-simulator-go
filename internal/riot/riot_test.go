package riot

import "testing"

func TestPortDDRMasking(t *testing.T) {
	var p Port
	p.WriteDDR(0x0F) // low nibble output, high nibble input
	p.Write(0xAA)    // written value, only low nibble bits matter for output
	p.InputFunc = func() uint8 { return 0xF0 }

	got := p.Read()
	want := uint8(0xA) | 0xF0 // output bits (0xAA & 0x0F = 0x0A) | input bits (0xF0 & ^0x0F = 0xF0)
	if got != want {
		t.Fatalf("Read() = %02X, want %02X", got, want)
	}
}

func TestPortReadWithoutInputFunc(t *testing.T) {
	var p Port
	p.WriteDDR(0xFF) // all output
	p.Write(0x5A)
	if got := p.Read(); got != 0x5A {
		t.Fatalf("Read() = %02X, want 0x5A", got)
	}
}

func TestTimerCountdownPerDivisor(t *testing.T) {
	cases := []struct {
		divider uint16
	}{
		{1}, {8}, {64}, {1024},
	}
	for _, tc := range cases {
		var tm Timer
		tm.Write(tc.divider, false, 5) // start at 5, count down to 0
		// value decrements once per `divider` cycles, so after exactly
		// 5*divider cycles it reaches 0 without having underflowed yet.
		tm.Tick(int(tc.divider) * 5)
		if tm.PeekValue() != 0 {
			t.Fatalf("divider=%d: value = %d, want 0 just before underflow", tc.divider, tm.PeekValue())
		}
		if tm.Underflowed() {
			t.Fatalf("divider=%d: should not have underflowed yet", tc.divider)
		}
		// One more full interval triggers the underflow.
		tm.Tick(int(tc.divider))
		if !tm.Underflowed() {
			t.Fatalf("divider=%d: should have underflowed", tc.divider)
		}
	}
}

func TestTimerDecrementsOncePerDivider(t *testing.T) {
	var tm Timer
	tm.Write(8, false, 10)
	tm.Tick(8)
	if tm.PeekValue() != 9 {
		t.Fatalf("after 8 cycles at divider 8: value = %d, want 9", tm.PeekValue())
	}
	tm.Tick(8 * 9)
	if tm.PeekValue() != 0 {
		t.Fatalf("value = %d, want 0", tm.PeekValue())
	}
}

func TestTimerReadClearsUnderflowFlag(t *testing.T) {
	var tm Timer
	tm.Write(1, false, 0)
	tm.Tick(1) // immediate underflow: value 0 -> flagged, wraps to 0xFF
	if !tm.Underflowed() {
		t.Fatalf("expected underflow")
	}
	if f := tm.ReadInterruptFlag(); f&0x80 == 0 {
		t.Fatalf("interrupt flag bit7 should be set")
	}
	_ = tm.ReadValue() // side effect: clears the flag
	if tm.Underflowed() {
		t.Fatalf("ReadValue should have cleared the underflow flag")
	}
	if f := tm.ReadInterruptFlag(); f&0x80 != 0 {
		t.Fatalf("interrupt flag should be clear after ReadValue")
	}
}

func TestTimerFreeRunsAfterUnderflow(t *testing.T) {
	var tm Timer
	tm.Write(1, false, 0)
	tm.Tick(1) // underflow -> wraps to 0xFF, free-runs at /1
	if tm.PeekValue() != 0xFF {
		t.Fatalf("value = %02X, want 0xFF after underflow wrap", tm.PeekValue())
	}
	tm.Tick(10)
	if tm.PeekValue() != 0xF5 {
		t.Fatalf("value = %02X, want 0xF5 after free-running 10 more cycles at /1", tm.PeekValue())
	}
}

func TestLoadROMValidatesSize(t *testing.T) {
	r := New()
	if err := r.LoadROM(make([]byte, 100)); err == nil {
		t.Fatalf("expected error loading wrong-size ROM")
	}
	if err := r.LoadROM(make([]byte, ROMSize)); err != nil {
		t.Fatalf("unexpected error loading correctly-sized ROM: %v", err)
	}
}

func TestRAMReadWrite(t *testing.T) {
	r := New()
	r.WriteRAM(0x10, 0x42)
	if got := r.ReadRAM(0x10); got != 0x42 {
		t.Fatalf("ReadRAM = %02X, want 0x42", got)
	}
	// offset must wrap within 64 bytes
	r.WriteRAM(0x40, 0x99) // 0x40 & 0x3F = 0x00
	if got := r.ReadRAM(0x00); got != 0x99 {
		t.Fatalf("ReadRAM(0x00) after WriteRAM(0x40) = %02X, want 0x99 (wraps)", got)
	}
}
