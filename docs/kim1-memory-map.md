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
| `$2000–$FFF9` | Unpopulated on a stock board; optionally backed by RAM (`System.EnableExpansionRAM`, `-ram-expansion`), simulating a RAM expansion board on the expansion connector |
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

**Verified by direct observation against real ROM dumps** (bringing up
`cmd/kim1` against real `6530-002.bin`/`6530-003.bin` images, pressing
each keypad (row, column) position programmatically and reading what the
monitor entered on the display):

- Electrical scan-matrix key → (row, column): row 0 columns 0-6 are hex
  digits 6,5,4,3,2,1,0; row 1 columns 0-6 are D,C,B,A,9,8,7; row 2 is
  PC,GO,+,DA,AD,F,E. See `internal/webui/static/app.js`'s `ELECTRICAL`
  table, the single place this mapping lives. (AD and PC were initially
  swapped — confirmed by testing that pressing AD after DA correctly
  routes further digit entry back into the address field, which only
  works with this assignment.)
- Segment bit→letter assignment (bit0=a … bit6=g) matches the monitor
  ROM's own hex-digit segment table byte-for-byte once bit 7 is masked
  off, e.g. `$BF"&0x7F=0x3F` for "0" (segments a-f on, g off).
- Physical (visual) key layout: 4 columns x 6 rows — GO/ST/RS/SST, then
  AD/DA/PC/+, then hex digits C-F / 8-B / 4-7 / 0-3 — matches a
  photographed real board (`internal/webui/static/app.js`'s
  `VISUAL_LAYOUT`), which is unrelated to the electrical scan-matrix
  layout above; the UI maps each legend from one to the other.

- GO: confirmed working — AD, enter an address, deposit a program via DA
  (e.g. `4C 00 02` = `JMP $0200` at $0200, a self-loop that makes success
  trivial to observe as PC staying put), AD the same address again, GO:
  PC lands on and stays at the entered address. (An earlier note here
  claimed GO's target-address handling was unverified/inconsistent; that
  was a bug in the *test script's* byte-deposit sequence — depositing "4"
  then "C" without checking the result actually wrote `$4D`, a different
  but still legal opcode, which is why the CPU didn't loop as expected.
  GO itself was never at fault.)

## TTY interface

**Verified by disassembling the real Kbd ROM dump's GETCH ($1E5A) and
OUTCH ($1EA0) routines** (`cmd/_scratchtty`-style scratch program, since
cleaned up) and cross-checking byte-for-byte against a reconstructed
source listing (brainwagon/kim-1):

- RX (into the CPU): Kbd RIOT **Port A bit 7**, idle-high (mark = 1,
  start bit = 0). Sampled directly by GETCH regardless of what the
  74145 keypad/display multiplexer is doing.
- TX (out of the CPU): Kbd RIOT **Port B bit 0**, same idle-high
  framing. OUTCH performs exactly one write per bit period (start, 8
  data bits LSB-first, stop), which is enough on its own to decode
  byte framing without needing to track timing at all — see
  `internal/kim1/tty.go`'s `ObserveTxWrite`.
- Port A bit 0 doubles as the physical TTY/keyboard mode switch
  (`System.TTYSelect`) whenever the multiplexer isn't actively
  selecting a keyboard row — read once at reset (`$1C31`) and again on
  every main-loop iteration (`$1C4F`) to decide whether to boot into
  TTY command mode or keypad/display mode.
- No baud-rate register exists: at reset, the monitor measures the
  width (in CPU cycles) of the first start bit it sees and reuses that
  as its per-bit delay for both directions from then on. This is why a
  real teletype's bootstrap procedure is to send RUBOUT ($7F) first —
  its data bits are all 1s (idle level), so the low pulse the monitor
  measures is exactly one bit wide. `System.Reset` automates this: with
  `TTYSelect` on, it auto-queues a RUBOUT send.
- End-to-end confirmed against the real ROM: after the above bootstrap,
  the decoded TTY output was exactly `"\r\n...KIM\r\n...0000 00 "` — the
  monitor's real boot banner and address/data prompt.
- Third-party TTY software can send bytes with the high bit set as
  non-printing filler, not just the monitor's own (always <$80) text.
  Confirmed against TinyBASIC-2000 (loaded via Load PAP to $2000 with
  expansion RAM enabled, entered via the TTY monitor's `G` command):
  raw captured bytes around its startup prompt were
  `0D FF FF 0A FF 3A 91` (traced bit-by-bit against the actual Port B
  write log, at the same ~3333-cycle/bit period the monitor itself
  uses — not a baud mismatch). Masking bit 7 turns `$FF` into RUBOUT
  (`$7F`) and `$91` into DC1 (`$11`), both non-printing ASCII control
  codes — i.e. this is TinyBASIC padding its output for a mechanical
  teletype's carriage-return timing, using the classic RUBOUT-as-filler
  convention (the same reason the boot bootstrap above sends one), just
  with the top bit set. A real teletype wouldn't print anything visible
  for these either, so the terminal panel masks bit 7 and drops
  non-printing control codes before rendering (`app.js`'s
  `cleanTTYText`) rather than showing raw mojibake; the underlying
  `ttyLog` byte stream itself is untouched.

## PAP paper-tape format and the memory viewer's Load PAP/Load HEX

The **Load PAP**/**Load HEX** buttons (`internal/kim1/loader.go`) parse a
file and write it straight into memory via `System.Write`, gated by
`Peek` the same way the memory editor's manual byte edits are (so a
record can't accidentally land on a RIOT I/O register and toggle a pin
or reset a timer). This is a separate, instant path from pasting a
`.pap` into the TTY panel, which instead feeds it through the real
simulated serial link and the monitor's own `L` command at whatever baud
rate `TTY.CyclesPerBit` is set to.

KIM-1 PAP records are `;LLAAAADD..DDCCCC`: `LL` = data byte count,
`AAAA` = address (high byte then low byte), `DD..DD` = `LL` data bytes,
`CCCC` = a 16-bit checksum. A record with `LL=$00` is the tape's
terminator.

**Checksum algorithm verified by disassembling the real App ROM's DUMP
routine** (`$1800`-ish) and Kbd ROM's LOAD routine: the running-checksum
subroutine at `$194C` is

```
TAY
CLC
ADC $17E7   ; checksum low byte
STA $17E7
LDA $17E8   ; checksum high byte
ADC #$00
STA $17E8
TYA
RTS
```

— an ordinary 16-bit sum with carry propagation into the high byte,
called once per decoded byte. Since addition is commutative, this means
`CCCC` is simply `(LL + addrHi + addrLo + sum(data)) mod 65536`
regardless of the order those bytes are added in.

Intel HEX (`Load HEX`) is a separate, standard, non-KIM-1-specific
format (`:LLAAAATTDD..DDCC`, two's-complement checksum) included because
it's a common assembler/linker output format; only data (`00`) and
end-of-file (`01`) record types are handled, since the KIM-1's 16-bit
address space has no use for the extended-address record types.

## Sources

- KIM-1 User's Manual V1.0 memory map (kim-1.com/docs/usrman.htm)
- Register address constants (PAD/PADD/PBD/PBDD at `$1700`; SAD/../CLK1T/
  CLK8T/CLK64T/CLKKT at `$1740`; NMIV/RSTV/IRQV soft vectors; NMIENT/
  RSTENT/IRQENT ROM entry points) cross-referenced from a reconstructed
  KIM-1 monitor source listing (brainwagon/kim-1 on GitHub)
- 74145 keypad/display multiplexing architecture (6502.org "What is the
  KIM-1?" hardware overview)
- Key legend layout and segment table: derived empirically from real ROM
  behavior, not a secondary source (see above)
