// Package webui serves a browser UI for a running kim1.System: a virtual
// hex keypad and 7-segment display kept live over a WebSocket, meant to be
// opened via VS Code's built-in Simple Browser panel (no custom extension
// needed).
package webui

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"sync"
	"time"

	"6502/internal/cpu"
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

	// ttyOut is what the teletype's printer has printed: every byte
	// decoded from the PTR loop, including the hardware echo of typed
	// keys (see kim1.TTY), capped to the last maxTTYBuf bytes. Appended
	// to from onTTYByte, which fires synchronously from within stepBatch
	// with mu already held.
	ttyOut []byte

	// memViewAddr is the start of the memory window shown in the hex
	// viewer, shared by all clients (this is a single-user local tool).
	memViewAddr uint16
}

// memViewBytes is the size of the hex viewer's window: one 256-byte page.
const memViewBytes = 256

// maxTTYBuf caps how much printer output is retained/broadcast; only
// the tail is kept once exceeded.
const maxTTYBuf = 4096

// NewServer returns a Server driving sys, throttled to approximately the
// real KIM-1's 1MHz clock by default.
func NewServer(sys *kim1.System) *Server {
	s := &Server{
		sys:      sys,
		clients:  make(map[*client]struct{}),
		TargetHz: 1_000_000,
	}
	sys.TTY.OnByte = s.onTTYByte
	return s
}

// onTTYByte is TTY.OnByte: called synchronously from within stepBatch
// (which already holds mu), so it must not lock.
func (s *Server) onTTYByte(b byte) {
	s.ttyOut = append(s.ttyOut, b)
	if excess := len(s.ttyOut) - maxTTYBuf; excess > 0 {
		s.ttyOut = append(s.ttyOut[:0], s.ttyOut[excess:]...)
	}
}

// ttyString maps each raw printer byte to the rune of the same value, so
// high-bit bytes survive JSON encoding (as U+0080-U+00FF) instead of
// being replaced as invalid UTF-8; the browser masks them itself.
func ttyString(b []byte) string {
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return string(r)
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
	mux.HandleFunc("GET /snapshot.bin", s.handleSnapshotSave)
	mux.HandleFunc("POST /snapshot.bin", s.handleSnapshotLoad)
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
	// Snapshot (rather than the raw shadow buffer) blanks any digit the
	// monitor's multiplex loop has stopped refreshing — e.g. while the
	// CPU is spinning in the user's own JMP loop rather than the
	// monitor's idle loop — matching real hardware, where persistence of
	// vision only sustains a digit's image as long as scanning actually
	// continues. See kim1.Display.Snapshot.
	digits := s.sys.Display.Snapshot(s.sys.CPU.Cycles)
	instr := disassembleAtDisplayAddress(s.sys, digits)
	msg := stateMsg{
		Type:       "state",
		A:          s.sys.CPU.A,
		X:          s.sys.CPU.X,
		Y:          s.sys.CPU.Y,
		SP:         s.sys.CPU.SP,
		PC:         s.sys.CPU.PC,
		P:          s.sys.CPU.P,
		Cycles:     s.sys.CPU.Cycles,
		Digits:     digits,
		Halted:     s.halted,
		Error:      s.haltReason,
		SST:        s.sys.SST,
		Instr:      instr,
		AppPA:      portState{Value: s.sys.App.PortA.Read(), DDR: s.sys.App.PortA.ReadDDR()},
		AppPB:      portState{Value: s.sys.App.PortB.Read(), DDR: s.sys.App.PortB.ReadDDR()},
		KbdPA:      portState{Value: s.sys.Kbd.PortA.Read(), DDR: s.sys.Kbd.PortA.ReadDDR()},
		KbdPB:      portState{Value: s.sys.Kbd.PortB.Read(), DDR: s.sys.Kbd.PortB.ReadDDR()},
		TTYSelect:  s.sys.TTYSelect,
		TTYOut:     ttyString(s.ttyOut),
		AppSwitchA: s.sys.AppSwitchA,
		AppSwitchB: s.sys.AppSwitchB,
		MemAddr:    s.memViewAddr,
		Mem:        s.memWindow(),
		Trail:      s.trail(),
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

// handleLoad parses text as a PAP or Intel HEX image and writes it
// straight into system memory, bypassing the TTY entirely (unlike the
// existing "paste into the terminal after L" workflow, this is meant to
// be instant regardless of the emulated baud rate). Callers must already
// hold mu.
func (s *Server) handleLoad(format, text string) *loadResultMsg {
	var records []kim1.MemRecord
	var err error
	switch format {
	case "pap":
		records, err = kim1.ParsePAP(text)
	case "hex":
		records, err = kim1.ParseIntelHex(text)
	default:
		err = fmt.Errorf("unknown format %q", format)
	}
	if err != nil {
		return &loadResultMsg{Type: "loadresult", Message: err.Error()}
	}

	stats := s.sys.LoadRecords(records)
	if stats.Written == 0 {
		return &loadResultMsg{Type: "loadresult", Message: "nothing loaded: file was empty, or every address it targets is unmapped"}
	}

	s.memViewAddr = stats.MinAddr &^ (memViewBytes - 1)
	msg := fmt.Sprintf("loaded %d bytes at $%04X-$%04X", stats.Written, stats.MinAddr, stats.MaxAddr)
	if stats.Skipped > 0 {
		msg += fmt.Sprintf(" (%d bytes skipped: unmapped address)", stats.Skipped)
	}
	return &loadResultMsg{Type: "loadresult", OK: true, Message: msg}
}

// handleSnapshotSave serves the whole 64KB address space as a raw binary
// download (see kim1.System.Snapshot). Plain HTTP rather than the
// WebSocket, so the browser's own download handling does the saving.
func (s *Server) handleSnapshotSave(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	img := s.sys.Snapshot()
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="kim1-snapshot.bin"`)
	w.Write(img)
}

// handleSnapshotLoad restores a 64KB image posted as the raw request body
// into RAM, then resets the CPU: the image carries no CPU registers, so
// whatever was executing can't meaningfully continue, while the monitor
// comes back up with the user program's saved registers ($EF-$F5)
// restored along with the rest of RAM, ready for GO. Replies with a
// loadResultMsg, same as a WebSocket "load".
func (s *Server) handleSnapshotLoad(w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(io.LimitReader(r.Body, kim1.SnapshotSize+1))
	result := loadResultMsg{Type: "loadresult"}
	if err != nil {
		result.Message = err.Error()
	} else {
		s.mu.Lock()
		n, err := s.sys.RestoreSnapshot(data)
		if err != nil {
			result.Message = err.Error()
		} else {
			s.sys.Reset()
			s.halted = false
			s.haltReason = ""
			result.OK = true
			result.Message = fmt.Sprintf("snapshot restored (%d bytes of RAM), CPU reset", n)
		}
		s.mu.Unlock()
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// memWindow returns memViewBytes bytes starting at memViewAddr via
// side-effect-free Peek, with -1 for addresses that can't be shown (I/O
// registers, unpopulated space). Callers must hold mu.
func (s *Server) memWindow() []int {
	out := make([]int, memViewBytes)
	for i := range out {
		if v, ok := s.sys.Peek(s.memViewAddr + uint16(i)); ok {
			out[i] = int(v)
		} else {
			out[i] = -1
		}
	}
	return out
}

// peekBus adapts System.Peek to bus.Bus for read-only debug use, so
// disassembling never triggers I/O register read side effects.
type peekBus struct{ sys *kim1.System }

func (b peekBus) Read(addr uint16) uint8 { v, _ := b.sys.Peek(addr); return v }
func (b peekBus) Write(uint16, uint8)    {}

// trail returns the most recently executed instructions, newest first,
// each with its byte length. Callers must hold mu.
func (s *Server) trail() []trailPC {
	pcs := s.sys.RecentPCs()
	out := make([]trailPC, len(pcs))
	for i, pc := range pcs {
		_, n := cpu.Disassemble(peekBus{s.sys}, pc)
		out[i] = trailPC{Addr: pc, Len: n}
	}
	return out
}
