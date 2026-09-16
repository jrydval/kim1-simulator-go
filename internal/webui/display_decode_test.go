package webui

import (
	"testing"

	"6502/internal/bus"
)

func TestDecodeDisplayAddress(t *testing.T) {
	// segments for "0200": 0,2,0,0 -> 0x3F,0x5B,0x3F,0x3F (data digits ignored)
	digits := [6]uint8{0x3F, 0x5B, 0x3F, 0x3F, 0x00, 0x00}
	addr, ok := decodeDisplayAddress(digits)
	if !ok || addr != 0x0200 {
		t.Fatalf("decodeDisplayAddress = %04X, %v; want 0200, true", addr, ok)
	}
}

func TestDecodeDisplayAddressHexLetters(t *testing.T) {
	// "ABCD": A=0x77 B=0x7C C=0x39 D=0x5E
	digits := [6]uint8{0x77, 0x7C, 0x39, 0x5E, 0x00, 0x00}
	addr, ok := decodeDisplayAddress(digits)
	if !ok || addr != 0xABCD {
		t.Fatalf("decodeDisplayAddress = %04X, %v; want ABCD, true", addr, ok)
	}
}

func TestDecodeDisplayAddressUnrecognizedPattern(t *testing.T) {
	digits := [6]uint8{0x3F, 0x3F, 0x3F, 0x01 /* not a valid digit */, 0x00, 0x00}
	_, ok := decodeDisplayAddress(digits)
	if ok {
		t.Fatalf("expected ok=false for an unrecognized segment pattern")
	}
}

func TestDisassembleAtDisplayAddress(t *testing.T) {
	ram := bus.NewFlatRAM()
	ram.Load(0x0200, []byte{0xA9, 0x42}) // LDA #$42
	digits := [6]uint8{0x3F, 0x5B, 0x3F, 0x3F, 0x00, 0x00}
	got := disassembleAtDisplayAddress(ram, digits)
	if got != "0200:  LDA #$42" {
		t.Fatalf("disassembleAtDisplayAddress = %q, want %q", got, "0200:  LDA #$42")
	}
}

func TestDisassembleAtDisplayAddressUnknown(t *testing.T) {
	ram := bus.NewFlatRAM()
	digits := [6]uint8{0x00, 0x00, 0x00, 0x00, 0x00, 0x00} // all blank, unrecognized
	got := disassembleAtDisplayAddress(ram, digits)
	if got != "----:  ?" {
		t.Fatalf("disassembleAtDisplayAddress = %q, want %q", got, "----:  ?")
	}
}
