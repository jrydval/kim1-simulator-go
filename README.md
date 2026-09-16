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
  Code's built-in Simple Browser panel.

Not yet implemented: the TTY/cassette interface, a disassembler/debugger,
and state snapshot save/restore.

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

## ROM images

This repository does **not** include KIM-1 ROM dumps — they are
third-party copyrighted content. See
[testdata/README.md](testdata/README.md) for how to source your own and
where the Klaus Dormann test binary (which *is* included) comes from.

## License

No license has been chosen yet for the original code in this repository.
