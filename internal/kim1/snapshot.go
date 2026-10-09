package kim1

import "fmt"

// SnapshotSize is the size of a memory snapshot: the full 6502 address
// space, so the file is a plain binary image any hex dump or other
// emulator can read with byte N at address $N.
const SnapshotSize = 0x10000

// Snapshot returns the whole 64KB address space as seen by Peek. Addresses
// with no side-effect-free value (RIOT I/O registers, unpopulated space)
// are stored as $00. CPU registers aren't included; the monitor keeps the
// user program's saved registers in zero page ($EF-$F5) anyway, so they
// travel with the image.
func (s *System) Snapshot() []byte {
	out := make([]byte, SnapshotSize)
	for addr := range out {
		out[addr], _ = s.Peek(uint16(addr))
	}
	return out
}

// RestoreSnapshot writes a Snapshot image back into every writable RAM
// region (system RAM, both RIOTs' RAM, and expansion RAM including the
// $0400-$13FF hole if enabled),
// ignoring the image's ROM, I/O and unpopulated bytes. It returns the
// number of bytes written.
func (s *System) RestoreSnapshot(data []byte) (int, error) {
	if len(data) != SnapshotSize {
		return 0, fmt.Errorf("snapshot must be exactly %d bytes, got %d", SnapshotSize, len(data))
	}
	n := 0
	restore := func(start, end uint16) {
		for addr := int(start); addr <= int(end); addr++ {
			s.Write(uint16(addr), data[addr])
			n++
		}
	}
	restore(ramStart, ramEnd)
	restore(appRAMStart, appRAMEnd)
	restore(kbdRAMStart, kbdRAMEnd)
	if s.expRAM != nil {
		restore(holeRAMStart, holeRAMEnd)
		restore(expRAMStart, expRAMEnd)
	}
	return n, nil
}
