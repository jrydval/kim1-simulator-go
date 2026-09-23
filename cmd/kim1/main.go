// Command kim1 runs a full KIM-1 simulation and serves its display/keypad
// as a browser UI. Open the printed URL in VS Code's "Simple Browser: Show"
// command (or any browser).
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"6502/internal/kim1"
	"6502/internal/webui"
)

func main() {
	romApp := flag.String("rom-app", "", "path to the 6530-003 (application RIOT) ROM image, 1024 bytes")
	romKbd := flag.String("rom-kbd", "", "path to the 6530-002 (keypad/display RIOT) ROM image, 1024 bytes")
	addr := flag.String("addr", "localhost:6502", "address to serve the web UI on")
	hz := flag.Int("hz", 1_000_000, "emulated CPU clock speed in Hz (0 = unthrottled)")
	ramExpansion := flag.Bool("ram-expansion", false, "fill $2000-$FFF9 with RAM, simulating an expansion RAM board (off by default, matching stock hardware's open bus there)")
	flag.Parse()

	if *romApp == "" || *romKbd == "" {
		fmt.Fprintln(os.Stderr, "kim1: -rom-app and -rom-kbd are required (see testdata/README.md for how to source KIM-1 ROM dumps)")
		flag.Usage()
		os.Exit(2)
	}

	sys := kim1.New()
	if err := sys.LoadAppROM(*romApp); err != nil {
		log.Fatal(err)
	}
	if err := sys.LoadKbdROM(*romKbd); err != nil {
		log.Fatal(err)
	}
	if *ramExpansion {
		sys.EnableExpansionRAM()
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

	fmt.Printf("KIM-1 running at http://%s — open it in VS Code via \"Simple Browser: Show\"\n", *addr)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
