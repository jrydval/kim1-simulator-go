// Package sdcard models an SDHC card in SPI mode at the byte level: each
// call to Exchange is one full-duplex byte on the bus, the way a card
// sees it once something (see kim1's bit-banged SPI decoder) has
// assembled the clocked bits. Only what simple 6502 SD drivers actually
// use is implemented -- the CP/M-65 KIM-1 "sdcard" BIOS and its boot
// loader (derived from sdcard6502) send CMD0, CMD8, CMD55+ACMD41, then
// single-block CMD17 reads and CMD24 writes -- plus CMD58 and CMD16 for
// drivers that probe a little more. Anything else answers "illegal
// command". CRCs are never checked, like a real card in SPI mode with
// CRC checking off.
//
// The card is SDHC-style block addressed: a read/write argument is a
// 512-byte sector number, not a byte offset.
package sdcard

import (
	"fmt"
	"io"
	"log"
)

// SectorSize is the fixed SDHC block size.
const SectorSize = 512

// Image is the card's backing store, typically an *os.File holding a raw
// disk image.
type Image interface {
	io.ReaderAt
	io.WriterAt
}

// R1 response bits.
const (
	r1Idle       = 0x01
	r1IllegalCmd = 0x04
	r1AddrError  = 0x20
)

// Data tokens and responses.
const (
	tokenStartBlock = 0xFE
	dataAccepted    = 0x05 // xxx0_010_1
	dataWriteError  = 0x0D // xxx0_110_1
)

type state int

const (
	stIdle      state = iota // waiting for a command's start byte
	stCommand                // collecting the rest of a 6-byte command
	stWriteWait              // CMD24 accepted: waiting for the start token
	stWriteData              // receiving a CMD24 block (+ 2 CRC bytes)
)

// Card is one SD card. The zero value is not usable; use New.
type Card struct {
	img     Image
	sectors uint32 // image size in sectors; reads/writes beyond it fail

	state state
	cmd   [6]byte
	n     int // bytes collected into cmd, or into buf while writing

	idle   bool // in the post-CMD0 idle state, until ACMD41 completes init
	appCmd bool // last command was CMD55, so the next one is an ACMD

	writeSector uint32
	buf         [SectorSize + 2]byte // block being written, plus its CRC

	out []byte // queued response bytes; 0xFF (idle bus) once drained

	// Err records the first I/O error from the backing image, if any;
	// every one is also logged. The host only sees an error response.
	Err error
}

// New returns a card backed by img, which holds size bytes (only whole
// sectors are addressable).
func New(img Image, size int64) *Card {
	return &Card{img: img, sectors: uint32(size / SectorSize), idle: true}
}

// Sectors reports the card's capacity in 512-byte sectors.
func (c *Card) Sectors() uint32 { return c.sectors }

// Deselect is called when the host raises CS. A half-sent command is
// dropped, but already-queued response bytes are kept: the
// sdcard6502-derived drivers raise CS right after reading CMD8's R1 and
// only then clock in its four trailing R7 bytes.
func (c *Card) Deselect() {
	if c.state == stCommand {
		c.state = stIdle
	}
}

// Exchange clocks one byte each way: in is what the host sent on MOSI,
// and the result is what the card drove on MISO at the same time.
func (c *Card) Exchange(in byte) byte {
	out := byte(0xFF)
	if len(c.out) > 0 {
		out = c.out[0]
		c.out = c.out[1:]
	}

	switch c.state {
	case stIdle:
		// A command starts with 01xxxxxx; the host clocks 0xFF while
		// waiting on responses, which can't start one. A new command
		// abandons whatever the host left unread of the last response.
		if in&0xC0 == 0x40 {
			c.out = c.out[:0]
			c.cmd[0] = in
			c.n = 1
			c.state = stCommand
		}
	case stCommand:
		c.cmd[c.n] = in
		c.n++
		if c.n == len(c.cmd) {
			c.state = stIdle
			c.execute()
		}
	case stWriteWait:
		if in == tokenStartBlock {
			c.n = 0
			c.state = stWriteData
		}
	case stWriteData:
		c.buf[c.n] = in
		c.n++
		if c.n == len(c.buf) {
			c.state = stIdle
			c.finishWrite()
		}
	}
	return out
}

func (c *Card) r1() byte {
	if c.idle {
		return r1Idle
	}
	return 0
}

func (c *Card) execute() {
	index := c.cmd[0] & 0x3F
	arg := uint32(c.cmd[1])<<24 | uint32(c.cmd[2])<<16 | uint32(c.cmd[3])<<8 | uint32(c.cmd[4])
	app := c.appCmd
	c.appCmd = false

	// One byte of Ncr (command-to-response gap) before every response,
	// as real cards give at least one.
	c.out = append(c.out, 0xFF)

	switch {
	case index == 0: // GO_IDLE_STATE
		c.idle = true
		c.out = append(c.out, r1Idle)
	case index == 8: // SEND_IF_COND: echo the voltage range and check pattern
		c.out = append(c.out, c.r1(), 0x00, 0x00, byte(arg>>8)&0x0F, byte(arg))
	case index == 55: // APP_CMD
		c.appCmd = true
		c.out = append(c.out, c.r1())
	case app && index == 41: // SD_SEND_OP_COND: initialization finishes at once
		c.idle = false
		c.out = append(c.out, 0x00)
	case index == 58: // READ_OCR: powered up, CCS=1 (SDHC), 2.7-3.6V
		c.out = append(c.out, c.r1(), 0xC0, 0xFF, 0x80, 0x00)
	case index == 16: // SET_BLOCKLEN: SDHC blocks are fixed at 512 anyway
		c.out = append(c.out, c.r1())
	case index == 17: // READ_SINGLE_BLOCK
		c.read(arg)
	case index == 24: // WRITE_BLOCK
		if arg >= c.sectors {
			c.out = append(c.out, c.r1()|r1AddrError)
			return
		}
		c.writeSector = arg
		c.state = stWriteWait
		c.out = append(c.out, c.r1())
	default:
		c.out = append(c.out, c.r1()|r1IllegalCmd)
	}
}

func (c *Card) read(sector uint32) {
	if sector >= c.sectors {
		c.out = append(c.out, c.r1()|r1AddrError)
		return
	}
	block := make([]byte, SectorSize)
	if _, err := c.img.ReadAt(block, int64(sector)*SectorSize); err != nil {
		c.noteErr(fmt.Errorf("sdcard: reading sector %d: %w", sector, err))
		c.out = append(c.out, c.r1()|r1AddrError)
		return
	}
	c.out = append(c.out, c.r1(), 0xFF, tokenStartBlock)
	c.out = append(c.out, block...)
	c.out = append(c.out, 0xFF, 0xFF) // CRC, unchecked
}

func (c *Card) finishWrite() {
	resp := byte(dataAccepted)
	if _, err := c.img.WriteAt(c.buf[:SectorSize], int64(c.writeSector)*SectorSize); err != nil {
		c.noteErr(fmt.Errorf("sdcard: writing sector %d: %w", c.writeSector, err))
		resp = dataWriteError
	}
	// Data response, then one busy byte (MISO held low while
	// programming), then the bus goes idle again.
	c.out = append(c.out, resp, 0x00)
}

func (c *Card) noteErr(err error) {
	log.Print(err)
	if c.Err == nil {
		c.Err = err
	}
}
