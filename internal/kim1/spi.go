package kim1

import "6502/internal/sdcard"

// SD card wiring on the App RIOT's Port A ($1700), as used by CP/M-65's
// KIM-1 "sdcard" port and the sdcard6502 code it derives from: a generic
// Arduino SD adapter (with 5V-to-3.3V level shifting) bit-banged as an
// SPI mode 0 master. CS, SCK and MOSI are outputs; MISO is an input.
const (
	sdPinMISO = 1
	sdPinMOSI = 2
	sdPinSCK  = 3
	sdPinCS   = 4
)

// spiState is the bit-level side of the SPI bus: the card itself only
// ever sees whole bytes (see sdcard.Card.Exchange).
type spiState struct {
	cs, sck uint8 // last seen pin levels, for edge detection
	bit     int   // bits clocked so far in the current byte
	in      byte  // MOSI bits collected so far
	out     byte  // byte the card is shifting out on MISO
	miso    uint8
}

// AttachSDCard plugs card into App RIOT Port A. While attached, the card
// drives PA1 (MISO) and the App switch bank's bit 1 no longer reaches
// the pin.
func (s *System) AttachSDCard(card *sdcard.Card) {
	s.sd = card
	s.spi = spiState{cs: 1, out: 0xFF, miso: 1}
}

// clockSPI runs after every write to the App RIOT's I/O window, looking
// at the resulting Port A pin levels. In mode 0 the host samples MISO
// after each rising SCK edge, so the card presents bit 7-k of its output
// byte from rising edge k until the next one, and samples MOSI on that
// same edge. A pin not configured as an output floats high, like the
// adapter's pull-ups -- which keeps the card deselected until the host
// sets up the DDR.
func (s *System) clockSPI() {
	if s.sd == nil {
		return
	}
	p := &s.App.PortA
	cs, sck, mosi := pinLevel(p, sdPinCS), pinLevel(p, sdPinSCK), pinLevel(p, sdPinMOSI)
	sp := &s.spi

	if cs != sp.cs {
		sp.cs = cs
		sp.bit, sp.in = 0, 0
		if cs == 1 {
			s.sd.Deselect()
		}
	}
	rising := sck == 1 && sp.sck == 0
	sp.sck = sck
	if cs != 0 || !rising {
		return
	}

	sp.miso = (sp.out >> uint(7-sp.bit)) & 1
	sp.in = sp.in<<1 | mosi
	sp.bit++
	if sp.bit == 8 {
		sp.out = s.sd.Exchange(sp.in)
		sp.bit, sp.in = 0, 0
	}
}

// appPortAInput supplies the App RIOT's Port A input bits: the switch
// bank, with PA1 taken over by the SD card's MISO when one is attached
// (idle-high while it's deselected).
func (s *System) appPortAInput() uint8 {
	v := s.AppSwitchA
	if s.sd != nil {
		miso := uint8(1)
		if s.spi.cs == 0 {
			miso = s.spi.miso
		}
		v = v&^(1<<sdPinMISO) | miso<<sdPinMISO
	}
	return v
}
