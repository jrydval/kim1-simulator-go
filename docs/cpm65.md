# Running CP/M-65 in the simulator

[CP/M-65](https://github.com/davidgiven/cpm65) is a CP/M-like operating
system for the 6502. Its KIM-1 port boots from an SD card attached to the
App RIOT's Port A. The simulator emulates that card, so you can run it
here.

## 1. What you need

- **The KIM-1 ROM images** in `testdata/`, the same ones the simulator
  always uses. If you don't have them yet, run `scripts/get-roms.sh`.
- **The CP/M-65 boot loader and disk image** from `kim-1-sdcard.zip` in
  the [CP/M-65 development
  release](https://github.com/davidgiven/cpm65/releases/tag/dev):
  - `bootsd.pap`: a small loader you put into KIM-1 memory;
  - `diskimage.raw`: the 32MB SD card, holding CP/M-65 and its programs.

From the repository root, run:

```sh
scripts/get-cpm65.sh
```

It downloads both files into `testdata/cpm65/` and makes `cpm65.raw`, a
working copy of the disk. `testdata/cpm65/` is in `.gitignore`, so these
files never get committed.

## 2. Start the simulator

The simulator writes everything CP/M-65 saves straight into the disk image,
so it runs on the working copy and `diskimage.raw` stays a clean original:

```sh
go run ./cmd/kim1 -ram-expansion -sdcard=testdata/cpm65/cpm65.raw -baud=9600
```

What the extra flags do:

| Flag | Why CP/M-65 needs it |
|---|---|
| `-ram-expansion` | Adds RAM at `$0400`-`$13FF` and `$2000`-`$FFF9`. A stock KIM-1 has only 1KB. |
| `-sdcard=testdata/cpm65/cpm65.raw` | Plugs the disk image in as the SD card. Without this flag no card is attached. |
| `-baud=9600` | Speeds the terminal up from the default 300 baud, which is far too slow for CP/M. |

Open the printed URL (`http://localhost:6502`) in a browser.

To start over with a fresh disk, stop the simulator (Ctrl+C) and copy
`testdata/cpm65/diskimage.raw` over `testdata/cpm65/cpm65.raw` again (or
rerun `scripts/get-cpm65.sh`).

## 3. Boot CP/M-65

1. Turn on the **TTY/KB** switch above the terminal panel, then press
   **RS**. The terminal shows `KIM`.
2. In the Memory panel, click **Load PAP** and choose
   `testdata/cpm65/bootsd.pap`. The loader goes into memory at `$0200`.
3. Click into the terminal and type `0200 G`.

The CP/M-65 banner appears and then the `A>` prompt.

## 4. Use it

Type a command and press Enter, for example:

| Command | What it does |
|---|---|
| `DIR` | List the files on the disk |
| `TYPE HELLO.ASM` | Print a text file |
| `REN NEW.ASM=HELLO.ASM` | Rename a file |
| `ERA NEW.ASM` | Delete a file |
| `STAT` | Show disk usage |
| `ATBASIC` | Start BASIC |

To run a program, type its name without `.COM`. The disk also has an
assembler, Pascal, Forth and other tools; `DIR` lists them all. This
port has only a plain TTY console with no screen driver, so full-screen
programs won't display properly.

**Type one command at a time.** The console is the KIM-1's bit-banged
TTY, which only listens while CP/M-65 is waiting for input. Anything you
type or paste while it is busy can be lost, the same as on a real KIM-1.
Wait for `A>` before typing the next command.

**Getting back to the monitor:** press **RS**. CP/M-65 stays in RAM, so
`0200 G` boots it again; you don't need to reload `bootsd.pap`.

## Troubleshooting

- **Nothing happens after `0200 G`, or you see an error code.** Check that
  the simulator was started with both `-ram-expansion` and `-sdcard`.
- **Garbled characters in the terminal.** Press **RS** to let the monitor
  recalibrate to the baud rate. Don't use more than `-baud=9600`.
- **`KIM` doesn't appear after RS.** The TTY/KB switch must be on before
  you press RS.

For how the SD card is wired and emulated, see [CP/M-65 on an SD
card](../README.md#cpm-65-on-an-sd-card) in the README.
