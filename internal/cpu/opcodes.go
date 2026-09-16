package cpu

// AddrMode identifies a 6502 addressing mode.
type AddrMode int

const (
	modeImplied AddrMode = iota
	modeAccumulator
	modeImmediate
	modeZeroPage
	modeZeroPageX
	modeZeroPageY
	modeAbsolute
	modeAbsoluteX
	modeAbsoluteY
	modeIndirect
	modeIndexedIndirect // (zp,X)
	modeIndirectIndexed // (zp),Y
	modeRelative
)

// opcodeInfo describes one of the 256 possible opcode bytes. name=="" marks
// a byte with no defined (official) instruction.
type opcodeInfo struct {
	name      string
	mode      AddrMode
	cycles    uint8
	pageCycle bool // +1 cycle if operand address computation crosses a page
}

// opcodeTable is the standard 6502 official-opcode cycle/addressing-mode
// table. Illegal/undocumented opcodes are left zero-valued (name=="") and
// cause execute() to panic — the KIM-1 monitor ROM and the base Klaus
// Dormann functional test never use them.
var opcodeTable = [256]opcodeInfo{
	0x00: {"BRK", modeImplied, 7, false},
	0x01: {"ORA", modeIndexedIndirect, 6, false},
	0x05: {"ORA", modeZeroPage, 3, false},
	0x06: {"ASL", modeZeroPage, 5, false},
	0x08: {"PHP", modeImplied, 3, false},
	0x09: {"ORA", modeImmediate, 2, false},
	0x0A: {"ASL", modeAccumulator, 2, false},
	0x0D: {"ORA", modeAbsolute, 4, false},
	0x0E: {"ASL", modeAbsolute, 6, false},

	0x10: {"BPL", modeRelative, 2, false},
	0x11: {"ORA", modeIndirectIndexed, 5, true},
	0x15: {"ORA", modeZeroPageX, 4, false},
	0x16: {"ASL", modeZeroPageX, 6, false},
	0x18: {"CLC", modeImplied, 2, false},
	0x19: {"ORA", modeAbsoluteY, 4, true},
	0x1D: {"ORA", modeAbsoluteX, 4, true},
	0x1E: {"ASL", modeAbsoluteX, 7, false},

	0x20: {"JSR", modeAbsolute, 6, false},
	0x21: {"AND", modeIndexedIndirect, 6, false},
	0x24: {"BIT", modeZeroPage, 3, false},
	0x25: {"AND", modeZeroPage, 3, false},
	0x26: {"ROL", modeZeroPage, 5, false},
	0x28: {"PLP", modeImplied, 4, false},
	0x29: {"AND", modeImmediate, 2, false},
	0x2A: {"ROL", modeAccumulator, 2, false},
	0x2C: {"BIT", modeAbsolute, 4, false},
	0x2D: {"AND", modeAbsolute, 4, false},
	0x2E: {"ROL", modeAbsolute, 6, false},

	0x30: {"BMI", modeRelative, 2, false},
	0x31: {"AND", modeIndirectIndexed, 5, true},
	0x35: {"AND", modeZeroPageX, 4, false},
	0x36: {"ROL", modeZeroPageX, 6, false},
	0x38: {"SEC", modeImplied, 2, false},
	0x39: {"AND", modeAbsoluteY, 4, true},
	0x3D: {"AND", modeAbsoluteX, 4, true},
	0x3E: {"ROL", modeAbsoluteX, 7, false},

	0x40: {"RTI", modeImplied, 6, false},
	0x41: {"EOR", modeIndexedIndirect, 6, false},
	0x45: {"EOR", modeZeroPage, 3, false},
	0x46: {"LSR", modeZeroPage, 5, false},
	0x48: {"PHA", modeImplied, 3, false},
	0x49: {"EOR", modeImmediate, 2, false},
	0x4A: {"LSR", modeAccumulator, 2, false},
	0x4C: {"JMP", modeAbsolute, 3, false},
	0x4D: {"EOR", modeAbsolute, 4, false},
	0x4E: {"LSR", modeAbsolute, 6, false},

	0x50: {"BVC", modeRelative, 2, false},
	0x51: {"EOR", modeIndirectIndexed, 5, true},
	0x55: {"EOR", modeZeroPageX, 4, false},
	0x56: {"LSR", modeZeroPageX, 6, false},
	0x58: {"CLI", modeImplied, 2, false},
	0x59: {"EOR", modeAbsoluteY, 4, true},
	0x5D: {"EOR", modeAbsoluteX, 4, true},
	0x5E: {"LSR", modeAbsoluteX, 7, false},

	0x60: {"RTS", modeImplied, 6, false},
	0x61: {"ADC", modeIndexedIndirect, 6, false},
	0x65: {"ADC", modeZeroPage, 3, false},
	0x66: {"ROR", modeZeroPage, 5, false},
	0x68: {"PLA", modeImplied, 4, false},
	0x69: {"ADC", modeImmediate, 2, false},
	0x6A: {"ROR", modeAccumulator, 2, false},
	0x6C: {"JMP", modeIndirect, 5, false},
	0x6D: {"ADC", modeAbsolute, 4, false},
	0x6E: {"ROR", modeAbsolute, 6, false},

	0x70: {"BVS", modeRelative, 2, false},
	0x71: {"ADC", modeIndirectIndexed, 5, true},
	0x75: {"ADC", modeZeroPageX, 4, false},
	0x76: {"ROR", modeZeroPageX, 6, false},
	0x78: {"SEI", modeImplied, 2, false},
	0x79: {"ADC", modeAbsoluteY, 4, true},
	0x7D: {"ADC", modeAbsoluteX, 4, true},
	0x7E: {"ROR", modeAbsoluteX, 7, false},

	0x81: {"STA", modeIndexedIndirect, 6, false},
	0x84: {"STY", modeZeroPage, 3, false},
	0x85: {"STA", modeZeroPage, 3, false},
	0x86: {"STX", modeZeroPage, 3, false},
	0x88: {"DEY", modeImplied, 2, false},
	0x8A: {"TXA", modeImplied, 2, false},
	0x8C: {"STY", modeAbsolute, 4, false},
	0x8D: {"STA", modeAbsolute, 4, false},
	0x8E: {"STX", modeAbsolute, 4, false},

	0x90: {"BCC", modeRelative, 2, false},
	0x91: {"STA", modeIndirectIndexed, 6, false},
	0x94: {"STY", modeZeroPageX, 4, false},
	0x95: {"STA", modeZeroPageX, 4, false},
	0x96: {"STX", modeZeroPageY, 4, false},
	0x98: {"TYA", modeImplied, 2, false},
	0x99: {"STA", modeAbsoluteY, 5, false},
	0x9A: {"TXS", modeImplied, 2, false},
	0x9D: {"STA", modeAbsoluteX, 5, false},

	0xA0: {"LDY", modeImmediate, 2, false},
	0xA1: {"LDA", modeIndexedIndirect, 6, false},
	0xA2: {"LDX", modeImmediate, 2, false},
	0xA4: {"LDY", modeZeroPage, 3, false},
	0xA5: {"LDA", modeZeroPage, 3, false},
	0xA6: {"LDX", modeZeroPage, 3, false},
	0xA8: {"TAY", modeImplied, 2, false},
	0xA9: {"LDA", modeImmediate, 2, false},
	0xAA: {"TAX", modeImplied, 2, false},
	0xAC: {"LDY", modeAbsolute, 4, false},
	0xAD: {"LDA", modeAbsolute, 4, false},
	0xAE: {"LDX", modeAbsolute, 4, false},

	0xB0: {"BCS", modeRelative, 2, false},
	0xB1: {"LDA", modeIndirectIndexed, 5, true},
	0xB4: {"LDY", modeZeroPageX, 4, false},
	0xB5: {"LDA", modeZeroPageX, 4, false},
	0xB6: {"LDX", modeZeroPageY, 4, false},
	0xB8: {"CLV", modeImplied, 2, false},
	0xB9: {"LDA", modeAbsoluteY, 4, true},
	0xBA: {"TSX", modeImplied, 2, false},
	0xBC: {"LDY", modeAbsoluteX, 4, true},
	0xBD: {"LDA", modeAbsoluteX, 4, true},
	0xBE: {"LDX", modeAbsoluteY, 4, true},

	0xC0: {"CPY", modeImmediate, 2, false},
	0xC1: {"CMP", modeIndexedIndirect, 6, false},
	0xC4: {"CPY", modeZeroPage, 3, false},
	0xC5: {"CMP", modeZeroPage, 3, false},
	0xC6: {"DEC", modeZeroPage, 5, false},
	0xC8: {"INY", modeImplied, 2, false},
	0xC9: {"CMP", modeImmediate, 2, false},
	0xCA: {"DEX", modeImplied, 2, false},
	0xCC: {"CPY", modeAbsolute, 4, false},
	0xCD: {"CMP", modeAbsolute, 4, false},
	0xCE: {"DEC", modeAbsolute, 6, false},

	0xD0: {"BNE", modeRelative, 2, false},
	0xD1: {"CMP", modeIndirectIndexed, 5, true},
	0xD5: {"CMP", modeZeroPageX, 4, false},
	0xD6: {"DEC", modeZeroPageX, 6, false},
	0xD8: {"CLD", modeImplied, 2, false},
	0xD9: {"CMP", modeAbsoluteY, 4, true},
	0xDD: {"CMP", modeAbsoluteX, 4, true},
	0xDE: {"DEC", modeAbsoluteX, 7, false},

	0xE0: {"CPX", modeImmediate, 2, false},
	0xE1: {"SBC", modeIndexedIndirect, 6, false},
	0xE4: {"CPX", modeZeroPage, 3, false},
	0xE5: {"SBC", modeZeroPage, 3, false},
	0xE6: {"INC", modeZeroPage, 5, false},
	0xE8: {"INX", modeImplied, 2, false},
	0xE9: {"SBC", modeImmediate, 2, false},
	0xEA: {"NOP", modeImplied, 2, false},
	0xEC: {"CPX", modeAbsolute, 4, false},
	0xED: {"SBC", modeAbsolute, 4, false},
	0xEE: {"INC", modeAbsolute, 6, false},

	0xF0: {"BEQ", modeRelative, 2, false},
	0xF1: {"SBC", modeIndirectIndexed, 5, true},
	0xF5: {"SBC", modeZeroPageX, 4, false},
	0xF6: {"INC", modeZeroPageX, 6, false},
	0xF8: {"SED", modeImplied, 2, false},
	0xF9: {"SBC", modeAbsoluteY, 4, true},
	0xFD: {"SBC", modeAbsoluteX, 4, true},
	0xFE: {"INC", modeAbsoluteX, 7, false},
}
