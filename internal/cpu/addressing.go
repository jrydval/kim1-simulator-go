package cpu

// resolveOperand reads and consumes the operand bytes for the given
// addressing mode, advancing PC past them, and returns the effective
// address to operate on (meaningless for modeImplied/modeAccumulator) plus
// whether the address computation crossed a page boundary.
//
// For modeImmediate the "address" is simply where the immediate byte lives
// (PC before it is consumed) so callers can read it uniformly via c.read.
// For modeRelative the "address" is the branch target; branch instructions
// decide the page-cross cycle penalty themselves based on whether the
// branch is actually taken.
func (c *CPU) resolveOperand(mode AddrMode) (addr uint16, pageCrossed bool) {
	switch mode {
	case modeImplied, modeAccumulator:
		return 0, false

	case modeImmediate:
		addr = c.PC
		c.PC++
		return addr, false

	case modeZeroPage:
		addr = uint16(c.read(c.PC))
		c.PC++
		return addr, false

	case modeZeroPageX:
		addr = uint16(uint8(c.read(c.PC) + c.X))
		c.PC++
		return addr, false

	case modeZeroPageY:
		addr = uint16(uint8(c.read(c.PC) + c.Y))
		c.PC++
		return addr, false

	case modeAbsolute:
		addr = c.read16(c.PC)
		c.PC += 2
		return addr, false

	case modeAbsoluteX:
		base := c.read16(c.PC)
		c.PC += 2
		addr = base + uint16(c.X)
		return addr, (base & 0xFF00) != (addr & 0xFF00)

	case modeAbsoluteY:
		base := c.read16(c.PC)
		c.PC += 2
		addr = base + uint16(c.Y)
		return addr, (base & 0xFF00) != (addr & 0xFF00)

	case modeIndirect:
		ptr := c.read16(c.PC)
		c.PC += 2
		return c.readBug16(ptr), false

	case modeIndexedIndirect: // (zp,X)
		zp := c.read(c.PC) + c.X
		c.PC++
		lo := uint16(c.read(uint16(zp)))
		hi := uint16(c.read(uint16(zp + 1)))
		return hi<<8 | lo, false

	case modeIndirectIndexed: // (zp),Y
		zp := c.read(c.PC)
		c.PC++
		lo := uint16(c.read(uint16(zp)))
		hi := uint16(c.read(uint16(zp + 1)))
		base := hi<<8 | lo
		addr = base + uint16(c.Y)
		return addr, (base & 0xFF00) != (addr & 0xFF00)

	case modeRelative:
		offset := int8(c.read(c.PC))
		c.PC++
		return uint16(int32(c.PC) + int32(offset)), false
	}
	return 0, false
}
