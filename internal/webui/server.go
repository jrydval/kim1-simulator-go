// Package webui serves a browser UI for a running kim1.System: a virtual
// hex keypad and 7-segment display kept live over a WebSocket, meant to be
// opened via VS Code's built-in Simple Browser panel (no custom extension
// needed).
package webui

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"sync"
	"time"

	"6502/internal/kim1"
)

//go:embed static
var embeddedStatic embed.FS

// Server drives a kim1.System (stepping its CPU on a background goroutine)
// and exposes it to the browser over HTTP + WebSocket.
type Server struct {
	sys *kim1.System

	// mu guards all access to sys, since it's mutated by the CPU-running
	// goroutine and by incoming WebSocket messages (key presses, reset).
	mu sync.Mutex

	clientsMu sync.Mutex
	clients   map[*client]struct{}

	// TargetHz throttles the emulated CPU to approximate the real KIM-1's
	// ~1MHz clock; 0 means run unthrottled.
	TargetHz int

	// halted and haltReason record a CPU panic (e.g. an illegal opcode —
	// very possible once GO starts executing arbitrary/uninitialized
	// memory as a program). Without this, an unrecovered panic in the
	// background CPU goroutine would silently kill the whole process,
	// which looks exactly like the UI freezing. Cleared by RS (reset).
	halted     bool
	haltReason string
}

// NewServer returns a Server driving sys, throttled to approximately the
// real KIM-1's 1MHz clock by default.
func NewServer(sys *kim1.System) *Server {
	return &Server{
		sys:      sys,
		clients:  make(map[*client]struct{}),
		TargetHz: 1_000_000,
	}
}

// Handler returns the HTTP handler serving the embedded static UI and the
// WebSocket endpoint.
func (s *Server) Handler() http.Handler {
	staticFS, err := fs.Sub(embeddedStatic, "static")
	if err != nil {
		panic(err) // embed.FS is compiled in; this can't fail at runtime
	}
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(staticFS)))
	mux.HandleFunc("/ws", s.handleWS)
	return mux
}

// RunCPU steps the emulated CPU until ctx is done, throttled to
// approximately TargetHz.
//
// The KIM-1's display is multiplexed entirely in software: the monitor
// ROM lights one digit at a time in a tight loop, relying on the CPU
// running continuously so persistence of vision (and, here, the
// kim1.Display shadow buffer — see docs/kim1-memory-map.md) sees a
// complete, evenly-refreshed image. Pacing execution in large chunks
// (e.g. run 10,000 cycles, then sleep ~10ms) freezes the CPU mid-way
// through a multiplex pass for most of that sleep — since a full 6-digit
// pass takes only ~4000 cycles, a naive large-chunk throttle regularly
// stops with some digits lit and others already blanked, which a client
// snapshot then captures as a flickering, partially-blank display. Small,
// frequent batches (well under one digit's ~700-cycle dwell time) keep
// the CPU's "off" gaps short enough that many complete passes still
// happen between any two broadcast samples, matching real hardware.
func (s *Server) RunCPU(ctx context.Context) {
	if s.TargetHz <= 0 {
		s.runUnthrottled(ctx)
		return
	}
	const batch = 100 // cycles per slice, well under one display digit's dwell time
	start := time.Now()
	var executed uint64

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		spent := s.stepBatch(batch)
		if spent == 0 {
			time.Sleep(50 * time.Millisecond) // halted: avoid busy-spinning until Reset
			continue
		}
		executed += uint64(spent)

		targetElapsed := time.Duration(float64(executed) / float64(s.TargetHz) * float64(time.Second))
		if lag := targetElapsed - time.Since(start); lag > 0 {
			time.Sleep(lag)
		}
	}
}

func (s *Server) runUnthrottled(ctx context.Context) {
	const batch = 100_000
	for {
		select {
		case <-ctx.Done():
			return
		default:
			if s.stepBatch(batch) == 0 {
				time.Sleep(50 * time.Millisecond) // halted: avoid busy-spinning until Reset
			}
		}
	}
}

// stepBatch runs up to n cycles' worth of CPU steps under the lock,
// recovering from (and logging) any panic — such as an illegal opcode —
// rather than letting it kill the whole process. Once halted, it's a
// no-op until Reset clears the flag. Returns the cycles actually spent.
func (s *Server) stepBatch(minCycles int) (spent int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.halted {
		return 0
	}
	defer func() {
		if r := recover(); r != nil {
			s.halted = true
			s.haltReason = fmt.Sprint(r)
			log.Printf("kim1: CPU halted: %v (press RS to reset)", r)
		}
	}()
	for spent < minCycles {
		spent += s.sys.Step()
	}
	return spent
}

// BroadcastLoop pushes the current system state to all connected
// WebSocket clients at a fixed, UI-friendly rate, independent of how fast
// the emulated CPU itself is running.
func (s *Server) BroadcastLoop(ctx context.Context) {
	const rate = time.Second / 30
	ticker := time.NewTicker(rate)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.broadcastState()
		}
	}
}

func (s *Server) broadcastState() {
	s.mu.Lock()
	instr := disassembleAtDisplayAddress(s.sys, s.sys.Display.Digits)
	msg := stateMsg{
		Type:   "state",
		A:      s.sys.CPU.A,
		X:      s.sys.CPU.X,
		Y:      s.sys.CPU.Y,
		SP:     s.sys.CPU.SP,
		PC:     s.sys.CPU.PC,
		P:      s.sys.CPU.P,
		Cycles: s.sys.CPU.Cycles,
		Digits: s.sys.Display.Digits,
		Halted: s.halted,
		Error:  s.haltReason,
		SST:    s.sys.SST,
		Instr:  instr,
	}
	s.mu.Unlock()

	s.clientsMu.Lock()
	defer s.clientsMu.Unlock()
	for c := range s.clients {
		select {
		case c.send <- msg:
		default: // slow client: drop this frame rather than block the broadcaster
		}
	}
}
