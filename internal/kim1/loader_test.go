package kim1

import "testing"

func TestParsePAPSingleRecord(t *testing.T) {
	// LL=01, addr=$0200, data=[$42]; checksum = 01+02+00+42 = 0045
	// (hand-computed, not round-tripped through our own encoder).
	records, err := ParsePAP(";010200420045")
	if err != nil {
		t.Fatalf("ParsePAP: %v", err)
	}
	if len(records) != 1 || records[0].Addr != 0x0200 || len(records[0].Data) != 1 || records[0].Data[0] != 0x42 {
		t.Fatalf("records = %+v, want [{0200 [42]}]", records)
	}
}

func TestParsePAPMultipleRecordsAndTerminator(t *testing.T) {
	// Two 2-byte records at $0300/$0302, then a terminator (LL=00).
	// $0300: 11 22 -> sum = 02+03+00+11+22 = 38
	// $0302: 33 44 -> sum = 02+03+02+33+44 = 7E
	text := ";0203001122" + "0038" + "\n" +
		";0203023344" + "007E" + "\n" +
		";0000000000\n"
	records, err := ParsePAP(text)
	if err != nil {
		t.Fatalf("ParsePAP: %v", err)
	}
	want := []MemRecord{
		{Addr: 0x0300, Data: []byte{0x11, 0x22}},
		{Addr: 0x0302, Data: []byte{0x33, 0x44}},
	}
	if len(records) != len(want) {
		t.Fatalf("records = %+v, want %+v", records, want)
	}
	for i := range want {
		if records[i].Addr != want[i].Addr || string(records[i].Data) != string(want[i].Data) {
			t.Fatalf("records[%d] = %+v, want %+v", i, records[i], want[i])
		}
	}
}

func TestParsePAPChecksumMismatch(t *testing.T) {
	if _, err := ParsePAP(";010200420046"); err == nil {
		t.Fatalf("expected a checksum-mismatch error")
	}
}

func TestParsePAPRejectsMalformed(t *testing.T) {
	cases := []string{
		"not a pap line",
		";01",                  // too short
		";ZZ020042000045",      // non-hex length field
		";FF02004200" + "0146", // claims 255 data bytes but has none
	}
	for _, c := range cases {
		if _, err := ParsePAP(c); err == nil {
			t.Fatalf("ParsePAP(%q): expected an error, got none", c)
		}
	}
}

func TestParseIntelHexDataAndEOF(t *testing.T) {
	// LL=02, addr=$0100, type=00, data=[$21,$46]; checksum computed by
	// hand: two's complement of (02+01+00+00+21+46) = 6A -> 96.
	text := ":02010000214696\n:00000001FF\n"
	records, err := ParseIntelHex(text)
	if err != nil {
		t.Fatalf("ParseIntelHex: %v", err)
	}
	if len(records) != 1 || records[0].Addr != 0x0100 || string(records[0].Data) != "\x21\x46" {
		t.Fatalf("records = %+v, want [{0100 [21 46]}]", records)
	}
}

func TestParseIntelHexStopsAtEOF(t *testing.T) {
	// A record after the EOF marker must be ignored.
	text := ":00000001FF\n:02010000214696\n"
	records, err := ParseIntelHex(text)
	if err != nil {
		t.Fatalf("ParseIntelHex: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("records = %+v, want none (EOF record seen first)", records)
	}
}

func TestParseIntelHexChecksumMismatch(t *testing.T) {
	if _, err := ParseIntelHex(":02010000214697"); err == nil {
		t.Fatalf("expected a checksum-mismatch error")
	}
}

func TestParseIntelHexRejectsExtendedAddress(t *testing.T) {
	// Type 04 (extended linear address); checksum irrelevant to the test
	// since the type is rejected before a mismatch would even matter --
	// use a value that does satisfy the checksum so the type check is
	// what actually fires.
	if _, err := ParseIntelHex(":02000004000A" + "F0"); err == nil {
		t.Fatalf("expected extended-address records to be rejected")
	}
}

func TestLoadRecordsSkipsUnmappedAndReportsRange(t *testing.T) {
	s := New()
	records := []MemRecord{
		{Addr: 0x0200, Data: []byte{0x11, 0x22, 0x33}},
		{Addr: 0x9000, Data: []byte{0x44}}, // unmapped without EnableExpansionRAM
	}
	stats := s.LoadRecords(records)
	if stats.Written != 3 || stats.Skipped != 1 {
		t.Fatalf("stats = %+v, want Written=3 Skipped=1", stats)
	}
	if stats.MinAddr != 0x0200 || stats.MaxAddr != 0x0202 {
		t.Fatalf("stats range = %04X-%04X, want 0200-0202", stats.MinAddr, stats.MaxAddr)
	}
	if v, _ := s.Peek(0x0200); v != 0x11 {
		t.Fatalf("Peek($0200) = %02X, want 11", v)
	}
	if v, _ := s.Peek(0x0202); v != 0x33 {
		t.Fatalf("Peek($0202) = %02X, want 33", v)
	}
}

func TestLoadRecordsIntoExpansionRAM(t *testing.T) {
	s := New()
	s.EnableExpansionRAM()
	stats := s.LoadRecords([]MemRecord{{Addr: 0x9000, Data: []byte{0xAA, 0xBB}}})
	if stats.Written != 2 || stats.Skipped != 0 {
		t.Fatalf("stats = %+v, want Written=2 Skipped=0", stats)
	}
	if v, _ := s.Peek(0x9000); v != 0xAA {
		t.Fatalf("Peek($9000) = %02X, want AA", v)
	}
}

func TestLoadRecordsDoesNotHitIORegisters(t *testing.T) {
	s := New()
	before := s.App.PortA.ReadDDR()
	// $1701 is the App RIOT's Port A DDR register; a record landing there
	// must not be applied, matching the memory-editor's own write-gate.
	stats := s.LoadRecords([]MemRecord{{Addr: 0x1701, Data: []byte{0xFF}}})
	if stats.Written != 0 || stats.Skipped != 1 {
		t.Fatalf("stats = %+v, want Written=0 Skipped=1", stats)
	}
	if got := s.App.PortA.ReadDDR(); got != before {
		t.Fatalf("Port A DDR changed from %02X to %02X: I/O register was written", before, got)
	}
}
