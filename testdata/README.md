# testdata

## `6502_functional_test.bin`

Klaus Dormann's well-known 6502 functional test suite, used as the golden
correctness test for the CPU core (`internal/cpu/functest_test.go`).

- Source: https://github.com/Klaus2m5/6502_65C02_functional_tests
- Author: Klaus Dormann, Copyright (C) 2012-2020
- License: GNU GPLv3 (see the header of `6502_functional_test.a65` in the
  source repository) — included here as a freestanding test-data binary
  used only to drive `go test`, not linked into or distributed as part of
  the emulator binary itself.
- This exact binary (assembled to load at `$0000`, entry point `$0400`)
  traps at `$3469` on success; any other trap address is a failing
  sub-test (cross-reference the assembly listing on GitHub for that
  address).

If this file is ever missing, re-download it from the `bin_files/`
directory of the repository above, or reassemble `6502_functional_test.a65`
yourself; `go test ./internal/cpu/...` skips `TestFunctional` gracefully
when the file isn't present.

## KIM-1 ROM images (not included)

The two KIM-1 6530 RIOT mask-ROM dumps (1KB each) are third-party
copyrighted content and are **not** included in this repository. To run
the full `cmd/kim1` system you must supply your own dumps via the
`-rom-app` / `-rom-kbd` flags:

- `-rom-app`: the 6530-003 (application connector RIOT), loaded at
  `$1800`
- `-rom-kbd`: the 6530-002 (keypad/display/TTY/cassette RIOT), loaded at
  `$1C00`

See [retro.hansotten.nl's KIM-1 ROMs
page](http://retro.hansotten.nl/6502-sbc/kim-1-manuals-and-software/kim_1-roms/)
for background on these ROM images and where to find dumps.
