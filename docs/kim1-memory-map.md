# KIM-1 memory map

Cross-checked against the published KIM-1 User's Manual memory map and
register-address constants used by reconstructed/disassembled monitor ROM
source listings (not reconstructed from memory alone — see sources at the
bottom). Implemented in `internal/kim1/memmap.go` and `internal/kim1/system.go`.

## Address ranges

| Range | Contents |
|---|---|
| `$0000–$03FF` | System RAM (1KB) |
| `$0400–$16FF` | Unpopulated on a stock board (reads as 0, writes ignored) |
| `$1700–$173F` | **App RIOT** (6530-003, user application connector) I/O + timer registers |
| `$1740–$177F` | **Kbd RIOT** (6530-002, keypad/display/TTY/cassette) I/O + timer registers |
| `$1780–$17BF` | App RIOT internal RAM (64 bytes) |
| `$17C0–$17FF` | Kbd RIOT internal RAM (64 bytes) — holds the NMIV/RSTV/IRQV "soft vectors" at `$17FA`/`$17FC`/`$17FE` |
| `$1800–$1BFF` | App RIOT ROM (1KB) |
| `$1C00–$1FFF` | Kbd RIOT ROM (1KB) — holds the hardware vector entry points at `$1FFA`/`$1FFC`/`$1FFE` |
| `$2000–$FFF9` | Unpopulated on a stock board |
| `$FFFA–$FFFF` | Aliased to the top of Kbd RIOT ROM (`$1FFA–$1FFF`) — incomplete address decoding on the real board is what makes RESET/IRQ/NMI reach the monitor ROM at all |

## RIOT I/O register decode

Each RIOT decodes only its low 3 address bits (`offset & 7`) within its
64-byte I/O window, mirrored 8 times — the standard 6530/6532 RIOT
pattern (8 registers = 3 bits):

| offset&7 | Register |
|---|---|
| 0 | Port A data |
| 1 | Port A DDR |
| 2 | Port B data |
| 3 | Port B DDR |
| 4 | Timer write ÷1 / Timer value read (clears underflow flag) |
| 5 | Timer write ÷8 / Interrupt flag read |
| 6 | Timer write ÷64 / Timer value read |
| 7 | Timer write ÷1024 / Interrupt flag read |

## Keypad / display multiplexing

Port B bits 1-4 of the Kbd RIOT feed an external 74145 BCD-to-decimal
decoder. Its outputs select one of 10 lines:

- Lines 0–2: keyboard row select (Port A, in input mode, reads that row's
  columns active-low)
- Line 3: unused
- Lines 4–9: display digit select (digit index = line − 4); Port A, in
  output mode, drives that digit's segments

**Not yet verified against a schematic:** the exact bit→segment (a–g)
assignment on Port A, and the exact physical key→(row,column) layout.
`internal/kim1/display.go` stores the raw 7-bit segment pattern rather
than decoded segment names specifically so this remaining uncertainty is
isolated to the presentation layer.

## Sources

- KIM-1 User's Manual V1.0 memory map (kim-1.com/docs/usrman.htm)
- Register address constants (PAD/PADD/PBD/PBDD at `$1700`; SAD/../CLK1T/
  CLK8T/CLK64T/CLKKT at `$1740`; NMIV/RSTV/IRQV soft vectors; NMIENT/
  RSTENT/IRQENT ROM entry points) cross-referenced from a reconstructed
  KIM-1 monitor source listing (brainwagon/kim-1 on GitHub)
- 74145 keypad/display multiplexing architecture (6502.org "What is the
  KIM-1?" hardware overview)
