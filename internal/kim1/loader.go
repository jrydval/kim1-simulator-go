package kim1

import (
	"fmt"
	"strconv"
	"strings"
)

// MemRecord is one contiguous block of bytes destined for a specific
// address, as parsed from a paper-tape or hex-file image.
type MemRecord struct {
	Addr uint16
	Data []byte
}

// ParsePAP parses a KIM-1 paper-tape (PAP) image: one
// ";LLAAAADD..DDCCCC" record per line, where LL is the data byte count,
// AAAA is the load address (high byte then low byte), DD..DD are LL data
// bytes, and CCCC is a 16-bit checksum. A record with LL=$00 is the
// tape's terminator and contributes no bytes.
//
// The checksum algorithm (plain 16-bit little-endian sum with carry
// propagation, so LL+addrHi+addrLo+each data byte can be added in any
// order) was confirmed against the real Kbd ROM's LOAD routine: its
// running-checksum subroutine at $194C is exactly
// TAY;CLC;ADC $17E7;STA $17E7;LDA $17E8;ADC #$00;STA $17E8;TYA;RTS —
// an ordinary 16-bit add with carry into the high byte, called once per
// decoded byte.
func ParsePAP(text string) ([]MemRecord, error) {
	var records []MemRecord
	for lineNo, raw := range strings.Split(text, "\n") {
		lineNo++ // 1-based for error messages
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if line[0] != ';' {
			return nil, fmt.Errorf("line %d: expected a record starting with ';', got %q", lineNo, line)
		}
		body := line[1:]
		if len(body) < 10 { // LL(2) + AAAA(4) + CCCC(4), minimum for an empty record
			return nil, fmt.Errorf("line %d: record too short", lineNo)
		}
		ll, err := hexByte(body[0:2])
		if err != nil {
			return nil, fmt.Errorf("line %d: bad length field: %w", lineNo, err)
		}
		need := 2 + 4 + int(ll)*2 + 4
		if len(body) < need {
			return nil, fmt.Errorf("line %d: record says %d data bytes but is too short for them", lineNo, ll)
		}
		addrHi, err := hexByte(body[2:4])
		if err != nil {
			return nil, fmt.Errorf("line %d: bad address field: %w", lineNo, err)
		}
		addrLo, err := hexByte(body[4:6])
		if err != nil {
			return nil, fmt.Errorf("line %d: bad address field: %w", lineNo, err)
		}
		sum := uint16(ll) + uint16(addrHi) + uint16(addrLo)
		data := make([]byte, ll)
		for i := range data {
			b, err := hexByte(body[6+i*2 : 8+i*2])
			if err != nil {
				return nil, fmt.Errorf("line %d: bad data byte %d: %w", lineNo, i, err)
			}
			data[i] = b
			sum += uint16(b)
		}
		chkOff := 6 + int(ll)*2
		chk, err := hexWord(body[chkOff : chkOff+4])
		if err != nil {
			return nil, fmt.Errorf("line %d: bad checksum field: %w", lineNo, err)
		}
		if chk != sum {
			return nil, fmt.Errorf("line %d: checksum mismatch (record says %04X, computed %04X)", lineNo, chk, sum)
		}
		if ll == 0 {
			continue // terminator record
		}
		records = append(records, MemRecord{Addr: uint16(addrHi)<<8 | uint16(addrLo), Data: data})
	}
	return records, nil
}

// ParseIntelHex parses a standard Intel HEX image: one
// ":LLAAAATTDD..DDCC" record per line, where CC is a two's-complement
// checksum (the sum of every byte in the record, including CC itself,
// is 0 mod 256). Only data (type 00) and end-of-file (type 01) records
// are meaningful for the KIM-1's 16-bit address space; extended-address
// records (02/04), which retarget the address field above 64KB, are
// rejected with a clear error instead of silently misplacing data.
func ParseIntelHex(text string) ([]MemRecord, error) {
	var records []MemRecord
	for lineNo, raw := range strings.Split(text, "\n") {
		lineNo++
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if line[0] != ':' {
			return nil, fmt.Errorf("line %d: expected a record starting with ':', got %q", lineNo, line)
		}
		body := line[1:]
		if len(body)%2 != 0 {
			return nil, fmt.Errorf("line %d: odd number of hex digits", lineNo)
		}
		if len(body) < 10 { // LL(2)+AAAA(4)+TT(2)+CC(2), minimum for an empty record
			return nil, fmt.Errorf("line %d: record too short", lineNo)
		}
		raw2 := make([]byte, len(body)/2)
		for i := range raw2 {
			b, err := hexByte(body[2*i : 2*i+2])
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", lineNo, err)
			}
			raw2[i] = b
		}
		ll := raw2[0]
		need := 5 + int(ll) // LL+AAAA(2)+TT+data+CC
		if len(raw2) != need {
			return nil, fmt.Errorf("line %d: record says %d data bytes but has %d bytes total", lineNo, ll, len(raw2))
		}
		var sum byte
		for _, b := range raw2 {
			sum += b
		}
		if sum != 0 {
			return nil, fmt.Errorf("line %d: checksum mismatch", lineNo)
		}
		addr := uint16(raw2[1])<<8 | uint16(raw2[2])
		typ := raw2[3]
		data := raw2[4 : 4+ll]
		switch typ {
		case 0x00:
			records = append(records, MemRecord{Addr: addr, Data: append([]byte(nil), data...)})
		case 0x01:
			return records, nil // end-of-file
		case 0x02, 0x04:
			return nil, fmt.Errorf("line %d: extended-address records aren't supported (the KIM-1 only has a 16-bit address space)", lineNo)
		default:
			return nil, fmt.Errorf("line %d: unsupported record type %02X", lineNo, typ)
		}
	}
	return records, nil
}

func hexByte(s string) (byte, error) {
	v, err := strconv.ParseUint(s, 16, 8)
	return byte(v), err
}

func hexWord(s string) (uint16, error) {
	v, err := strconv.ParseUint(s, 16, 16)
	return uint16(v), err
}

// LoadStats summarizes a LoadRecords call: how many bytes actually
// landed in memory, how many were skipped, and the address range the
// written bytes span (meaningful only if Written > 0).
type LoadStats struct {
	Written int
	Skipped int
	MinAddr uint16
	MaxAddr uint16
}

// LoadRecords writes each record's bytes via Write, but only into
// addresses that are safely Peek-able (system/expansion RAM, RIOT
// internal RAM, ROM aliasing) — the same rule the memory viewer/editor's
// manual byte edits follow, so a record that happens to target a RIOT
// I/O register can't sneak in a port write or reset a timer's countdown
// as a side effect of a bulk load. Addresses outside any of that (e.g.
// $2000+ without EnableExpansionRAM) are silently skipped, same as
// writing there directly would be.
func (s *System) LoadRecords(records []MemRecord) LoadStats {
	var st LoadStats
	for _, r := range records {
		for i, b := range r.Data {
			addr := r.Addr + uint16(i)
			if _, ok := s.Peek(addr); !ok {
				st.Skipped++
				continue
			}
			s.Write(addr, b)
			if st.Written == 0 || addr < st.MinAddr {
				st.MinAddr = addr
			}
			if st.Written == 0 || addr > st.MaxAddr {
				st.MaxAddr = addr
			}
			st.Written++
		}
	}
	return st
}
