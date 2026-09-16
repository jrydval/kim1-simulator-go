package cpu

import "fmt"

// execute decodes and runs the single instruction at PC, returning the
// number of cycles it consumed (including any page-cross/branch penalty).
func (c *CPU) execute() int {
	opcode := c.read(c.PC)
	info := opcodeTable[opcode]
	if info.name == "" {
		panic(fmt.Sprintf("cpu: illegal/unimplemented opcode $%02X at $%04X", opcode, c.PC))
	}
	pc := c.PC
	c.PC++

	addr, pageCrossed := c.resolveOperand(info.mode)

	cycles := int(info.cycles)
	if info.pageCycle && pageCrossed {
		cycles++
	}
	cycles += c.dispatch(info.name, info.mode, addr, pc)
	return cycles
}

// dispatch performs the instruction's semantics and returns any extra
// cycles not already accounted for by the static table (currently only
// branch-taken / branch-page-cross penalties).
func (c *CPU) dispatch(name string, mode AddrMode, addr uint16, instrPC uint16) int {
	switch name {

	// --- Loads / Stores ---
	case "LDA":
		c.A = c.read(addr)
		c.setZN(c.A)
	case "LDX":
		c.X = c.read(addr)
		c.setZN(c.X)
	case "LDY":
		c.Y = c.read(addr)
		c.setZN(c.Y)
	case "STA":
		c.write(addr, c.A)
	case "STX":
		c.write(addr, c.X)
	case "STY":
		c.write(addr, c.Y)

	// --- Transfers ---
	case "TAX":
		c.X = c.A
		c.setZN(c.X)
	case "TAY":
		c.Y = c.A
		c.setZN(c.Y)
	case "TXA":
		c.A = c.X
		c.setZN(c.A)
	case "TYA":
		c.A = c.Y
		c.setZN(c.A)
	case "TSX":
		c.X = c.SP
		c.setZN(c.X)
	case "TXS":
		c.SP = c.X

	// --- Stack ---
	case "PHA":
		c.push(c.A)
	case "PHP":
		c.push(c.P | Flag5 | FlagB)
	case "PLA":
		c.A = c.pop()
		c.setZN(c.A)
	case "PLP":
		c.P = (c.pop() &^ FlagB) | Flag5

	// --- Logic ---
	case "AND":
		c.A &= c.read(addr)
		c.setZN(c.A)
	case "ORA":
		c.A |= c.read(addr)
		c.setZN(c.A)
	case "EOR":
		c.A ^= c.read(addr)
		c.setZN(c.A)
	case "BIT":
		v := c.read(addr)
		c.setFlag(FlagZ, c.A&v == 0)
		c.setFlag(FlagV, v&0x40 != 0)
		c.setFlag(FlagN, v&0x80 != 0)

	// --- Arithmetic ---
	case "ADC":
		c.adc(c.read(addr))
	case "SBC":
		c.sbc(c.read(addr))
	case "CMP":
		c.compare(c.A, c.read(addr))
	case "CPX":
		c.compare(c.X, c.read(addr))
	case "CPY":
		c.compare(c.Y, c.read(addr))

	// --- Shifts / Rotates ---
	case "ASL":
		c.shiftRotate(mode, addr, func(v uint8) uint8 {
			c.setFlag(FlagC, v&0x80 != 0)
			return v << 1
		})
	case "LSR":
		c.shiftRotate(mode, addr, func(v uint8) uint8 {
			c.setFlag(FlagC, v&0x01 != 0)
			return v >> 1
		})
	case "ROL":
		c.shiftRotate(mode, addr, func(v uint8) uint8 {
			carryIn := uint8(0)
			if c.flag(FlagC) {
				carryIn = 1
			}
			c.setFlag(FlagC, v&0x80 != 0)
			return v<<1 | carryIn
		})
	case "ROR":
		c.shiftRotate(mode, addr, func(v uint8) uint8 {
			carryIn := uint8(0)
			if c.flag(FlagC) {
				carryIn = 0x80
			}
			c.setFlag(FlagC, v&0x01 != 0)
			return v>>1 | carryIn
		})

	// --- Increment / Decrement ---
	case "INC":
		v := c.read(addr) + 1
		c.write(addr, v)
		c.setZN(v)
	case "DEC":
		v := c.read(addr) - 1
		c.write(addr, v)
		c.setZN(v)
	case "INX":
		c.X++
		c.setZN(c.X)
	case "INY":
		c.Y++
		c.setZN(c.Y)
	case "DEX":
		c.X--
		c.setZN(c.X)
	case "DEY":
		c.Y--
		c.setZN(c.Y)

	// --- Jumps / Calls ---
	case "JMP":
		c.PC = addr
	case "JSR":
		c.push16(c.PC - 1)
		c.PC = addr
	case "RTS":
		c.PC = c.pop16() + 1
	case "BRK":
		c.PC++ // BRK's second byte is a padding/signature byte, skipped on return
		c.handleInterrupt(irqVector, true)
	case "RTI":
		c.P = (c.pop() &^ FlagB) | Flag5
		c.PC = c.pop16()

	// --- Branches ---
	case "BPL":
		return c.branch(!c.flag(FlagN), addr)
	case "BMI":
		return c.branch(c.flag(FlagN), addr)
	case "BVC":
		return c.branch(!c.flag(FlagV), addr)
	case "BVS":
		return c.branch(c.flag(FlagV), addr)
	case "BCC":
		return c.branch(!c.flag(FlagC), addr)
	case "BCS":
		return c.branch(c.flag(FlagC), addr)
	case "BNE":
		return c.branch(!c.flag(FlagZ), addr)
	case "BEQ":
		return c.branch(c.flag(FlagZ), addr)

	// --- Flags ---
	case "CLC":
		c.setFlag(FlagC, false)
	case "SEC":
		c.setFlag(FlagC, true)
	case "CLI":
		c.setFlag(FlagI, false)
	case "SEI":
		c.setFlag(FlagI, true)
	case "CLV":
		c.setFlag(FlagV, false)
	case "CLD":
		c.setFlag(FlagD, false)
	case "SED":
		c.setFlag(FlagD, true)

	case "NOP":
		// no-op

	default:
		panic(fmt.Sprintf("cpu: unhandled instruction %q at $%04X", name, instrPC))
	}
	return 0
}

func (c *CPU) shiftRotate(mode AddrMode, addr uint16, op func(uint8) uint8) {
	if mode == modeAccumulator {
		c.A = op(c.A)
		c.setZN(c.A)
		return
	}
	v := op(c.read(addr))
	c.write(addr, v)
	c.setZN(v)
}

func (c *CPU) compare(reg, value uint8) {
	result := reg - value
	c.setFlag(FlagC, reg >= value)
	c.setZN(result)
}

// branch applies the taken/not-taken and page-crossing cycle penalties.
// c.PC at call time already points at the instruction following the
// branch, which is the correct reference point for the page-cross check.
func (c *CPU) branch(cond bool, target uint16) int {
	if !cond {
		return 0
	}
	extra := 1
	if c.PC&0xFF00 != target&0xFF00 {
		extra++
	}
	c.PC = target
	return extra
}

// adc implements binary and NMOS-decimal-mode addition, including the
// documented BCD quirks: the Z flag reflects the binary sum, while N/V
// reflect the sum after the low-nibble decimal adjustment but before the
// high-nibble one.
func (c *CPU) adc(value uint8) {
	a := c.A
	var carryIn uint8
	if c.flag(FlagC) {
		carryIn = 1
	}
	binSum := uint16(a) + uint16(value) + uint16(carryIn)

	if !c.flag(FlagD) {
		result := uint8(binSum)
		c.setFlag(FlagV, (a^value)&0x80 == 0 && (a^result)&0x80 != 0)
		c.setFlag(FlagC, binSum > 0xFF)
		c.A = result
		c.setZN(result)
		return
	}

	al := (a & 0x0F) + (value & 0x0F) + carryIn
	if al >= 0x0A {
		al = ((al + 0x06) & 0x0F) + 0x10
	}
	preAdjust := uint16(a&0xF0) + uint16(value&0xF0) + uint16(al)
	preAdjustByte := uint8(preAdjust)
	c.setFlag(FlagN, preAdjustByte&0x80 != 0)
	c.setFlag(FlagV, (a^value)&0x80 == 0 && (a^preAdjustByte)&0x80 != 0)
	if preAdjust >= 0xA0 {
		preAdjust += 0x60
	}
	c.setFlag(FlagC, preAdjust >= 0x100)
	c.setFlag(FlagZ, uint8(binSum) == 0)
	c.A = uint8(preAdjust)
}

// sbc implements binary and NMOS-decimal-mode subtraction. Per documented
// NMOS behavior, all flags (N/V/Z/C) always follow binary-subtraction
// semantics even in decimal mode — only the stored accumulator digits get
// BCD-adjusted.
func (c *CPU) sbc(value uint8) {
	a := c.A
	inv := ^value
	var carryIn uint8
	if c.flag(FlagC) {
		carryIn = 1
	}
	sum := uint16(a) + uint16(inv) + uint16(carryIn)
	result := uint8(sum)
	c.setFlag(FlagV, (a^inv)&0x80 == 0 && (a^result)&0x80 != 0)
	c.setFlag(FlagC, sum > 0xFF)
	c.setZN(result)

	if !c.flag(FlagD) {
		c.A = result
		return
	}

	al := int16(a&0x0F) - int16(value&0x0F) - int16(1-carryIn)
	if al < 0 {
		al = ((al - 0x06) & 0x0F) - 0x10
	}
	diff := int16(a&0xF0) - int16(value&0xF0) + al
	if diff < 0 {
		diff -= 0x60
	}
	c.A = uint8(diff)
}
