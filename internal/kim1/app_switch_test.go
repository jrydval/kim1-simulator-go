package kim1

import "testing"

const (
	appPAData = appIOStart + 0
	appPADDR  = appIOStart + 1
	appPBData = appIOStart + 2
	appPBDDR  = appIOStart + 3
)

func TestAppSwitchAffectsInputPin(t *testing.T) {
	s := New()
	s.Write(appPADDR, 0x00) // Port A all input

	s.SetAppSwitch('A', 3, true)
	if got := s.Read(appPAData); got != 1<<3 {
		t.Fatalf("Read(App PA) = %02X, want %02X (switch bit 3 on)", got, uint8(1<<3))
	}

	s.SetAppSwitch('A', 3, false)
	if got := s.Read(appPAData); got != 0 {
		t.Fatalf("Read(App PA) after clearing = %02X, want 0", got)
	}
}

func TestAppSwitchIgnoredOnOutputPin(t *testing.T) {
	s := New()
	s.Write(appPADDR, 0xFF) // Port A all output
	s.Write(appPAData, 0x00)

	s.SetAppSwitch('A', 0, true)
	if got := s.Read(appPAData); got != 0 {
		t.Fatalf("Read(App PA) = %02X, want 0 (bit 0 is an output, switch shouldn't affect it)", got)
	}
}

func TestAppSwitchPortB(t *testing.T) {
	s := New()
	s.Write(appPBDDR, 0x00)

	s.SetAppSwitch('B', 7, true)
	if got := s.Read(appPBData); got != 1<<7 {
		t.Fatalf("Read(App PB) = %02X, want %02X", got, uint8(1<<7))
	}
}

func TestAppSwitchOutOfRangeIgnored(t *testing.T) {
	s := New()
	s.Write(appPADDR, 0x00)
	s.SetAppSwitch('A', 8, true)  // out of range
	s.SetAppSwitch('A', -1, true) // out of range
	s.SetAppSwitch('C', 0, true)  // invalid port

	if got := s.Read(appPAData); got != 0 {
		t.Fatalf("Read(App PA) = %02X, want 0 (all out-of-range calls should be no-ops)", got)
	}
}
