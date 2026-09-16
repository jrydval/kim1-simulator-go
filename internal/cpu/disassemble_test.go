package cpu

import (
	"testing"

	"6502/internal/bus"
)

func TestDisassemble(t *testing.T) {
	cases := []struct {
		bytes  []byte
		want   string
		length int
	}{
		{[]byte{0xA9, 0x42}, "LDA #$42", 2},
		{[]byte{0xA5, 0x10}, "LDA $10", 2},
		{[]byte{0xB5, 0x10}, "LDA $10,X", 2},
		{[]byte{0xAD, 0x00, 0x02}, "LDA $0200", 3},
		{[]byte{0xBD, 0x00, 0x02}, "LDA $0200,X", 3},
		{[]byte{0xB9, 0x00, 0x02}, "LDA $0200,Y", 3},
		{[]byte{0x4C, 0x00, 0x1C}, "JMP $1C00", 3},
		{[]byte{0x6C, 0x00, 0x1C}, "JMP ($1C00)", 3},
		{[]byte{0xA1, 0x20}, "LDA ($20,X)", 2},
		{[]byte{0xB1, 0x20}, "LDA ($20),Y", 2},
		{[]byte{0x0A}, "ASL A", 1},
		{[]byte{0xEA}, "NOP", 1},
		{[]byte{0x00}, "BRK", 1},
		{[]byte{0x02}, ".byte $02", 1}, // illegal opcode
	}

	for _, c := range cases {
		ram := bus.NewFlatRAM()
		ram.Load(0x0200, c.bytes)
		text, length := Disassemble(ram, 0x0200)
		if text != c.want || length != c.length {
			t.Errorf("Disassemble(% X) = %q, %d; want %q, %d", c.bytes, text, length, c.want, c.length)
		}
	}
}

func TestDisassembleRelativeBranch(t *testing.T) {
	ram := bus.NewFlatRAM()
	ram.Load(0x0200, []byte{0xF0, 0x02}) // BEQ +2 -> target $0204
	text, length := Disassemble(ram, 0x0200)
	if text != "BEQ $0204" || length != 2 {
		t.Errorf("Disassemble(BEQ) = %q, %d; want %q, 2", text, length, "BEQ $0204")
	}
}
