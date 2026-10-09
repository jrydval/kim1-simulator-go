package kim1

import (
	"testing"

	"6502/internal/sdcard"
)

// memImage is an in-memory sdcard.Image.
type memImage []byte

func (m memImage) ReadAt(p []byte, off int64) (int, error)  { return copy(p, m[off:]), nil }
func (m memImage) WriteAt(p []byte, off int64) (int, error) { return copy(m[off:], p), nil }

const (
	csBit   = 1 << sdPinCS
	sckBit  = 1 << sdPinSCK
	mosiBit = 1 << sdPinMOSI
	misoBit = 1 << sdPinMISO
)

// spiByte bit-bangs one byte through App Port A the way CP/M-65's libsd
// does (set MOSI with SCK low, raise SCK, sample MISO), with CS low.
func spiByte(s *System, out byte) byte {
	var in byte
	for i := 7; i >= 0; i-- {
		v := uint8(0)
		if out>>uint(i)&1 != 0 {
			v = mosiBit
		}
		s.Write(appIOStart, v)
		s.Write(appIOStart, v|sckBit)
		in <<= 1
		if s.Read(appIOStart)&misoBit != 0 {
			in |= 1
		}
	}
	return in
}

func TestSPIBitBangReachesCard(t *testing.T) {
	s := New()
	s.AttachSDCard(sdcard.New(make(memImage, sdcard.SectorSize), sdcard.SectorSize))
	s.Write(appIOStart+1, csBit|sckBit|mosiBit) // DDR: CS/SCK/MOSI out, MISO in

	if got := s.Read(appIOStart) & misoBit; got == 0 {
		t.Fatal("MISO should idle high while the card is deselected")
	}

	// 80 clocks with CS high must be ignored by the card.
	for i := 0; i < 160; i++ {
		s.Write(appIOStart, csBit|mosiBit|uint8(i%2)*sckBit)
	}

	for _, b := range []byte{0x40, 0, 0, 0, 0, 0x95} { // CMD0
		spiByte(s, b)
	}
	r1 := byte(0xFF)
	for i := 0; i < 10 && r1 == 0xFF; i++ {
		r1 = spiByte(s, 0xFF)
	}
	if r1 != 0x01 {
		t.Fatalf("CMD0 R1 over bit-banged SPI = %02X, want 01", r1)
	}
}

func TestSDCardOverridesOnlyMISOSwitch(t *testing.T) {
	s := New()
	s.AppSwitchA = 0xFF &^ misoBit
	s.AttachSDCard(sdcard.New(make(memImage, sdcard.SectorSize), sdcard.SectorSize))
	if got := s.Read(appIOStart); got != 0xFF {
		t.Fatalf("Port A = %02X, want FF (switches, plus idle-high MISO)", got)
	}
}
