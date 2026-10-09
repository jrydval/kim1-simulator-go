package sdcard

import (
	"bytes"
	"testing"
)

// memImage is an in-memory Image.
type memImage []byte

func (m memImage) ReadAt(p []byte, off int64) (int, error)  { return copy(p, m[off:]), nil }
func (m memImage) WriteAt(p []byte, off int64) (int, error) { return copy(m[off:], p), nil }

// command sends a 6-byte command and returns the first non-0xFF byte
// clocked back (the R1 response), like the 6502 drivers' sd_waitresult.
func command(c *Card, index byte, arg uint32) byte {
	for _, b := range []byte{0x40 | index, byte(arg >> 24), byte(arg >> 16), byte(arg >> 8), byte(arg), 0x01} {
		c.Exchange(b)
	}
	return waitResult(c)
}

func waitResult(c *Card) byte {
	for i := 0; i < 100; i++ {
		if r := c.Exchange(0xFF); r != 0xFF {
			return r
		}
	}
	return 0xFF
}

func initCard(t *testing.T, c *Card) {
	t.Helper()
	if r := command(c, 0, 0); r != 0x01 {
		t.Fatalf("CMD0 R1 = %02X, want 01", r)
	}
	if r := command(c, 8, 0x1AA); r != 0x01 {
		t.Fatalf("CMD8 R1 = %02X, want 01", r)
	}
	c.Deselect() // the KIM-1 drivers raise CS before reading the R7 tail
	var r7 [4]byte
	for i := range r7 {
		r7[i] = c.Exchange(0xFF)
	}
	if r7 != [4]byte{0x00, 0x00, 0x01, 0xAA} {
		t.Fatalf("CMD8 R7 tail = % X, want 00 00 01 AA", r7)
	}
	if r := command(c, 55, 0); r != 0x01 {
		t.Fatalf("CMD55 R1 = %02X, want 01", r)
	}
	if r := command(c, 41, 0x40000000); r != 0x00 {
		t.Fatalf("ACMD41 R1 = %02X, want 00 (initialized)", r)
	}
}

func TestReadAndWriteSector(t *testing.T) {
	img := make(memImage, 4*SectorSize)
	for i := range img[SectorSize : 2*SectorSize] {
		img[SectorSize+i] = byte(i)
	}
	c := New(img, int64(len(img)))
	initCard(t, c)

	if r := command(c, 17, 1); r != 0x00 {
		t.Fatalf("CMD17 R1 = %02X, want 00", r)
	}
	if tok := waitResult(c); tok != 0xFE {
		t.Fatalf("data token = %02X, want FE", tok)
	}
	got := make([]byte, SectorSize)
	for i := range got {
		got[i] = c.Exchange(0xFF)
	}
	if !bytes.Equal(got, img[SectorSize:2*SectorSize]) {
		t.Fatal("CMD17 returned the wrong data")
	}
	c.Exchange(0xFF) // CRC
	c.Exchange(0xFF)

	if r := command(c, 24, 3); r != 0x00 {
		t.Fatalf("CMD24 R1 = %02X, want 00", r)
	}
	c.Exchange(0xFE)
	for i := 0; i < SectorSize; i++ {
		c.Exchange(0x5A)
	}
	c.Exchange(0xFF) // CRC
	c.Exchange(0xFF)
	if resp := waitResult(c); resp&0x1F != 0x05 {
		t.Fatalf("data response = %02X, want xxx00101 (accepted)", resp)
	}
	for c.Exchange(0xFF) != 0xFF { // busy while programming
	}
	if !bytes.Equal(img[3*SectorSize:], bytes.Repeat([]byte{0x5A}, SectorSize)) {
		t.Fatal("CMD24 didn't write sector 3")
	}
	if c.Err != nil {
		t.Fatalf("Err = %v", c.Err)
	}
}

func TestOutOfRangeAndUnknownCommands(t *testing.T) {
	img := make(memImage, 2*SectorSize)
	c := New(img, int64(len(img)))
	initCard(t, c)

	if r := command(c, 17, 2); r&r1AddrError == 0 {
		t.Fatalf("CMD17 past the end: R1 = %02X, want the address error bit", r)
	}
	if r := command(c, 24, 2); r&r1AddrError == 0 {
		t.Fatalf("CMD24 past the end: R1 = %02X, want the address error bit", r)
	}
	if r := command(c, 9, 0); r&r1IllegalCmd == 0 {
		t.Fatalf("CMD9: R1 = %02X, want the illegal command bit", r)
	}
}

func TestOCRReportsSDHC(t *testing.T) {
	c := New(make(memImage, SectorSize), SectorSize)
	initCard(t, c)
	if r := command(c, 58, 0); r != 0x00 {
		t.Fatalf("CMD58 R1 = %02X, want 00", r)
	}
	if ocr0 := c.Exchange(0xFF); ocr0&0x40 == 0 {
		t.Fatalf("OCR byte 0 = %02X, want CCS (bit 30) set", ocr0)
	}
}
