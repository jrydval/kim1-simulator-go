package cpu

import (
	"os"
	"testing"

	"6502/internal/bus"
)

// Klaus Dormann's 6502 functional test (see testdata/README.md) loads at
// $0000, starts execution at $0400, and signals completion by branching to
// itself forever — a successful run traps at $3469, any other trap address
// indicates the specific sub-test that failed (cross-reference the .a65
// source's trap labels near that address).
const (
	functestLoadAddr    = 0x0000
	functestStartPC     = 0x0400
	functestSuccessTrap = 0x3469
	functestMaxSteps    = 100_000_000
)

func TestFunctional(t *testing.T) {
	path := "../../testdata/6502_functional_test.bin"
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("skipping: %s not found (see testdata/README.md to obtain it): %v", path, err)
	}

	ram := bus.NewFlatRAM()
	ram.Load(functestLoadAddr, data)
	c := New(ram)
	c.PC = functestStartPC

	var prevPC uint16
	for i := 0; i < functestMaxSteps; i++ {
		prevPC = c.PC
		c.Step()
		if c.PC == prevPC {
			if c.PC != functestSuccessTrap {
				t.Fatalf("functional test trapped at $%04X (expected success trap $%04X) after %d steps — see 6502_functional_test.lst for the failing sub-test at this address", c.PC, functestSuccessTrap, i)
			}
			return
		}
	}
	t.Fatalf("functional test did not trap within %d steps (PC=$%04X)", functestMaxSteps, c.PC)
}
