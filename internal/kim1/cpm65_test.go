package kim1

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"6502/internal/sdcard"
)

// TestCPM65BootsFromSDCard boots CP/M-65's KIM-1 "sdcard" port end to end
// -- real monitor ROM, bootsd.pap loader, the bit-banged SPI SD card --
// and runs DIR. Skipped unless the ROMs and testdata/cpm65/ (see
// testdata/README.md) are present.
func TestCPM65BootsFromSDCard(t *testing.T) {
	for _, baud := range []uint64{300, 2400, 9600} {
		t.Run(fmt.Sprint(baud), func(t *testing.T) { testCPM65Boot(t, baud) })
	}
}

func testCPM65Boot(t *testing.T, baud uint64) {
	s := newROMSystem(t)
	s.TTY.CyclesPerBit = 1_000_000 / baud
	pap, err := os.ReadFile("../../testdata/cpm65/bootsd.pap")
	if err != nil {
		t.Skipf("skipping: %v", err)
	}
	card := openCardCopy(t, "../../testdata/cpm65/diskimage.raw")

	s.EnableExpansionRAM()
	s.AttachSDCard(card)
	out := printer(s)
	s.TTYSelect = true
	s.Reset()
	runCycles(s, 100*charCycles)

	records, err := ParsePAP(string(pap))
	if err != nil {
		t.Fatalf("ParsePAP: %v", err)
	}
	s.LoadRecords(records)
	s.TTY.Send([]byte("0200 G")...)

	waitFor := func(want string, maxCycles uint64) {
		t.Helper()
		for end := s.CPU.Cycles + maxCycles; s.CPU.Cycles < end; {
			runCycles(s, 100_000)
			if strings.Contains(out.String(), want) {
				return
			}
		}
		t.Fatalf("no %q after %d cycles; printer shows %q (card err: %v)", want, maxCycles, out.String(), card.Err)
	}
	waitFor("A>", 200_000_000)

	out.Reset()
	s.TTY.Send([]byte("DIR\r")...)
	waitFor("A>", 200_000_000)
	if !strings.Contains(out.String(), "HELLO    ASM") {
		t.Fatalf("DIR output %q doesn't list HELLO.ASM", out.String())
	}

	// REN rewrites a directory sector: exercises CMD24.
	out.Reset()
	s.TTY.Send([]byte("REN WORLD.ASM=HELLO.ASM\r")...)
	waitFor("A>", 200_000_000)
	s.TTY.Send([]byte("DIR WORLD.ASM\r")...)
	waitFor("WORLD    ASM", 200_000_000)
	if card.Err != nil {
		t.Fatalf("card error: %v", card.Err)
	}
}

// openCardCopy returns a card backed by a private copy of the image at
// path, so tests never modify the original.
func openCardCopy(t *testing.T, path string) *sdcard.Card {
	t.Helper()
	src, err := os.Open(path)
	if err != nil {
		t.Skipf("skipping: %v", err)
	}
	defer src.Close()
	dst, err := os.Create(filepath.Join(t.TempDir(), "sd.img"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dst.Close() })
	size, err := io.Copy(dst, src)
	if err != nil {
		t.Fatal(err)
	}
	return sdcard.New(dst, size)
}
