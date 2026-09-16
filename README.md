# KIM-1 Simulator in Golang

A cycle-accurate MOS 6502 CPU emulator with a full simulation of the
[KIM-1](https://en.wikipedia.org/wiki/KIM-1) single-board computer
(MOS Technology, 1976): CPU, system RAM, both 6530 RIOT chips (I/O,
timer, and their mask ROMs), the 6-digit 7-segment display, and the
24-key hex keypad.

## Status

Implemented and tested (`go test ./...`):

- `internal/cpu` — full 6502 instruction set (all official opcodes and
  addressing modes), cycle-accurate timing, BCD arithmetic. Verified
  against Klaus Dormann's [6502 functional test
  suite](https://github.com/Klaus2m5/6502_65C02_functional_tests).
- `internal/riot` — MOS 6530 RIOT chip model (RAM, ROM, DDR-masked I/O
  ports, 4-divisor interval timer).
- `internal/kim1` — the full KIM-1 memory map and address decode wiring
  the CPU to system RAM and both RIOT chips, plus keypad/display
  multiplexing logic (modeling the external 74145 decoder).
- `internal/webui` + `cmd/kim1` — a Go web server exposing the running
  KIM-1's display and keypad over WebSocket, meant to be opened via VS
  Code's built-in Simple Browser panel, including a working SST
  (single-step) switch mirroring the real hardware's SYNC-driven NMI
  mechanism, a live disassembly (`internal/cpu/disassemble.go`) of
  the instruction at whatever address is shown on the display — i.e.
  what you're actually examining via `AD`/`DA`/`+`, not the CPU's own PC
  (almost always deep in idle monitor code) — shown next to the register
  debug panel, an I/O port panel showing both RIOTs' Port A/B pins as
  LEDs (lit = high, solid outline = output pin, dashed = input pin), and
  a TTY terminal panel (see below).

Not yet implemented: the cassette interface, breakpoints, and state
snapshot save/restore.

See [docs/kim1-memory-map.md](docs/kim1-memory-map.md) for the verified
address map and remaining open questions (exact segment-bit and
keypad-layout assignments).

## Building and testing

```sh
go build ./...
go test ./...
```

## Running

You need your own KIM-1 ROM dumps (see below). Then:

```sh
go run ./cmd/kim1 -rom-app=path/to/6530-003.bin -rom-kbd=path/to/6530-002.bin
```

This starts a web server (default `http://localhost:6502`). Open that URL
in a browser, or in VS Code via the **Simple Browser: Show** command —
there's also a "Run KIM-1" task in `.vscode/tasks.json` that prompts for
the two ROM paths and starts the server for you.

## Using the keypad: returning to the monitor from a program

`BRK` and `ST` only return control to the monitor if the interrupt
vectors in RIOT RAM have been set up first — this is standard KIM-1
behavior (per the original documentation), not something the monitor ROM
initializes automatically, and this emulator intentionally doesn't
special-case it either. Before relying on `BRK` to end a program:

1. `AD` → `17FE`, `DA` → deposit `00`, `+`, deposit `1C` (sets the IRQ
   vector to `$1C00`)
2. End your program with `BRK` (opcode `$00`) — it will now cleanly
   return to the monitor's address/data display.

If you also want to use `ST` (single-step/stop), set the NMI vector the
same way at `$17FA`/`$17FB`. Without either setup, both `BRK` and `ST`
hang instead of returning — `RS` (full reset) always works regardless,
but doesn't preserve registers.

## Single-stepping (SST)

The `SST` button in the web UI is a toggle mirroring the real KIM-1's
physical Single-Step slide switch. With it on, every instruction executed
from RAM (your program) — but not from ROM (the monitor itself) — is
followed by an NMI, handing control back to the monitor after exactly one
instruction. Requires the NMI vector to be set up first (see above).
Press `GO` repeatedly to step through your program one instruction at a
time; the monitor saves and restores registers across each step, so
watch the display (or single-step slowly) rather than expecting to catch
live CPU register values mid-step — the monitor's own display-scan loop
reuses those same registers between your steps.

`ST` only makes sense while a program is actually running between when
you press `GO` and when it would otherwise return to the monitor on its
own. Pressing `ST` while sitting in the monitor's idle loop — including
right after an `SST` step, since by the time you see the display update
control is already back in the monitor — interrupts the monitor itself
rather than your program, with no meaningful "where it stopped" state to
save, and shows a garbage ROM-ish address. With `SST` on, every `GO`
already stops after exactly one instruction, so there's never a useful
moment to also press `ST`; it's a tool for manually breaking into a
program that's running *without* `SST` (e.g. a long/infinite loop),
not a companion to it.

## TTY terminal

The `TTY/KB` switch in the web UI mirrors the real KIM-1's physical
TTY/keyboard mode jumper. With it on, the monitor boots into TTY command
mode instead of keypad/display mode; with it off (the default), the
keypad/display work as normal and the terminal panel stays unused.

The serial link is emulated at the pin level, not by trapping ROM
routines: `internal/kim1/tty.go` reconstructs whatever the CPU writes to
the TTY output pin (Port B bit 0) into bytes, and drives the TTY input
pin (Port A bit 7) with whatever you type, framed as a real 1-start /
8-data / 1-stop bit software UART — confirmed against the real ROM's
GETCH ($1E5A) and OUTCH ($1EA0) routines. The monitor has no baud-rate
setting; at reset it measures the width of the first start bit it
receives and uses that as its bit time from then on, which is why a real
teletype's bootstrap procedure is to send RUBOUT first. This emulator
does that step automatically — flip `TTY/KB` on, then press `RS`, and
the boot banner should appear in the terminal panel.

## ROM images

This repository does **not** include KIM-1 ROM dumps — they are
third-party copyrighted content. See
[testdata/README.md](testdata/README.md) for how to source your own and
where the Klaus Dormann test binary (which *is* included) comes from.

## License

No license has been chosen yet for the original code in this repository.
