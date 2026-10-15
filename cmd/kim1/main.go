// Command kim1 runs a full KIM-1 simulation and serves its display/keypad
// as a browser UI. Open the printed URL in VS Code's "Simple Browser: Show"
// command (or any browser).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"6502/internal/kim1"
	"6502/internal/sdcard"
	"6502/internal/webui"
)

func main() {
	romApp := flag.String("rom-app", "testdata/6530-003.bin", "path to the 6530-003 (application RIOT) ROM image, 1024 bytes")
	romKbd := flag.String("rom-kbd", "testdata/6530-002.bin", "path to the 6530-002 (keypad/display RIOT) ROM image, 1024 bytes")
	addr := flag.String("addr", "localhost:6502", "address to serve the web UI on")
	hz := flag.Int("hz", 1_000_000, "emulated CPU clock speed in Hz (0 = unthrottled)")
	ramExpansion := flag.Bool("ram-expansion", false, "fill $0400-$13FF and $2000-$FFF9 with RAM, simulating a full RAM upgrade (off by default, matching stock hardware's open bus there)")
	sdImage := flag.String("sdcard", "", "raw SD card image to attach via bit-banged SPI on App RIOT Port A (PA4=CS, PA3=SCK, PA2=MOSI, PA1=MISO), as used by CP/M-65; writes go straight to the file")
	baud := flag.Int("baud", 300, "TTY baud rate (the monitor auto-calibrates to it on reset; up to 9600 works)")
	flag.Parse()

	sys := kim1.New()
	for _, err := range []error{sys.LoadAppROM(*romApp), sys.LoadKbdROM(*romKbd)} {
		if errors.Is(err, fs.ErrNotExist) {
			log.Fatalf("%v\nRun scripts/get-roms.sh to download the ROM images into testdata/, or point -rom-app/-rom-kbd at your own.", err)
		}
		if err != nil {
			log.Fatal(err)
		}
	}
	if *ramExpansion {
		sys.EnableExpansionRAM()
	}
	if *baud <= 0 {
		log.Fatalf("kim1: -baud must be positive, got %d", *baud)
	}
	sys.TTY.CyclesPerBit = uint64(1_000_000 / *baud)
	if *sdImage != "" {
		f, err := os.OpenFile(*sdImage, os.O_RDWR, 0)
		if err != nil {
			log.Fatalf("kim1: opening SD card image: %v", err)
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil {
			log.Fatalf("kim1: opening SD card image: %v", err)
		}
		sys.AttachSDCard(sdcard.New(f, info.Size()))
	}
	sys.Reset()

	srv := webui.NewServer(sys)
	srv.TargetHz = *hz

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go srv.RunCPU(ctx)
	go srv.BroadcastLoop(ctx)

	httpServer := &http.Server{Addr: *addr, Handler: srv.Handler()}
	go func() {
		<-ctx.Done()
		_ = httpServer.Close()
	}()

	fmt.Printf("KIM-1 simulator started at http://%s\n", *addr)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
