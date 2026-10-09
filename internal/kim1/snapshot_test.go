package kim1

import "testing"

func TestSnapshotRoundTrip(t *testing.T) {
	src := New()
	src.EnableExpansionRAM()
	src.Write(0x0010, 0x11)
	src.Write(0x17C5, 0x22) // Kbd RIOT RAM
	src.Write(0x4000, 0x33) // expansion RAM
	src.Kbd.ROM[0] = 0x44

	img := src.Snapshot()
	if len(img) != SnapshotSize {
		t.Fatalf("len = %d, want %d", len(img), SnapshotSize)
	}
	if img[0x1C00] != 0x44 {
		t.Fatalf("img[$1C00] = %02X, want ROM contents 44", img[0x1C00])
	}

	dst := New()
	dst.EnableExpansionRAM()
	n, err := dst.RestoreSnapshot(img)
	if err != nil {
		t.Fatalf("RestoreSnapshot: %v", err)
	}
	if want := 0x400 + 0x80 + 0x1000 + int(expRAMEnd-expRAMStart+1); n != want {
		t.Fatalf("restored %d bytes, want %d", n, want)
	}
	for _, c := range []struct {
		addr uint16
		want uint8
	}{{0x0010, 0x11}, {0x17C5, 0x22}, {0x4000, 0x33}, {0x1C00, 0x00}} {
		if v, _ := dst.Peek(c.addr); v != c.want {
			t.Errorf("Peek($%04X) = %02X, want %02X", c.addr, v, c.want)
		}
	}
}

func TestRestoreSnapshotSkipsExpansionWhenDisabled(t *testing.T) {
	img := make([]byte, SnapshotSize)
	img[0x4000] = 0x55
	n, err := New().RestoreSnapshot(img)
	if err != nil {
		t.Fatalf("RestoreSnapshot: %v", err)
	}
	if n != 0x400+0x80 {
		t.Fatalf("restored %d bytes, want only on-board RAM", n)
	}
}

func TestRestoreSnapshotRejectsWrongSize(t *testing.T) {
	if _, err := New().RestoreSnapshot(make([]byte, 1024)); err == nil {
		t.Fatal("expected an error for a 1KB image")
	}
}
