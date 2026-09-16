package cpu

import (
	"fmt"

	"6502/internal/bus"
)

// Disassemble returns the 6502 assembly mnemonic text for the
// instruction at addr (e.g. "LDA #$42", "JMP $1C00") and its length in
// bytes (1-3). It reads through b the same way CPU.Step would, so it's
// safe to call speculatively (e.g. for a debug display) against any Bus
// backing real RAM/ROM; it has no side effects beyond the reads
// themselves.
func Disassemble(b bus.Bus, addr uint16) (text string, length int) {
	opcode := b.Read(addr)
	info := opcodeTable[opcode]
	if info.name == "" {
		return fmt.Sprintf(".byte $%02X", opcode), 1
	}

	operand := func(n uint16) uint16 { return addr + 1 + n }

	switch info.mode {
	case modeImplied:
		return info.name, 1
	case modeAccumulator:
		return info.name + " A", 1
	case modeImmediate:
		return fmt.Sprintf("%s #$%02X", info.name, b.Read(operand(0))), 2
	case modeZeroPage:
		return fmt.Sprintf("%s $%02X", info.name, b.Read(operand(0))), 2
	case modeZeroPageX:
		return fmt.Sprintf("%s $%02X,X", info.name, b.Read(operand(0))), 2
	case modeZeroPageY:
		return fmt.Sprintf("%s $%02X,Y", info.name, b.Read(operand(0))), 2
	case modeAbsolute:
		return fmt.Sprintf("%s $%04X", info.name, readAbs(b, operand(0))), 3
	case modeAbsoluteX:
		return fmt.Sprintf("%s $%04X,X", info.name, readAbs(b, operand(0))), 3
	case modeAbsoluteY:
		return fmt.Sprintf("%s $%04X,Y", info.name, readAbs(b, operand(0))), 3
	case modeIndirect:
		return fmt.Sprintf("%s ($%04X)", info.name, readAbs(b, operand(0))), 3
	case modeIndexedIndirect:
		return fmt.Sprintf("%s ($%02X,X)", info.name, b.Read(operand(0))), 2
	case modeIndirectIndexed:
		return fmt.Sprintf("%s ($%02X),Y", info.name, b.Read(operand(0))), 2
	case modeRelative:
		offset := int8(b.Read(operand(0)))
		target := uint16(int32(addr) + 2 + int32(offset))
		return fmt.Sprintf("%s $%04X", info.name, target), 2
	}
	return info.name, 1
}

func readAbs(b bus.Bus, addr uint16) uint16 {
	lo := uint16(b.Read(addr))
	hi := uint16(b.Read(addr + 1))
	return hi<<8 | lo
}
