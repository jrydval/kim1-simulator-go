package webui

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"6502/internal/kim1"
)

func TestSnapshotSaveAndLoadOverHTTP(t *testing.T) {
	src := kim1.New()
	src.Write(0x0200, 0x42)
	srcSrv := httptest.NewServer(NewServer(src).Handler())
	defer srcSrv.Close()

	resp, err := http.Get(srcSrv.URL + "/snapshot.bin")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	img, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if len(img) != kim1.SnapshotSize || img[0x0200] != 0x42 {
		t.Fatalf("got %d bytes, img[$0200] = %02X; want %d bytes with 42", len(img), img[0x0200], kim1.SnapshotSize)
	}

	dst := kim1.New()
	s := NewServer(dst)
	s.halted = true
	dstSrv := httptest.NewServer(s.Handler())
	defer dstSrv.Close()

	resp, err = http.Post(dstSrv.URL+"/snapshot.bin", "application/octet-stream", bytes.NewReader(img))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	var result loadResultMsg
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	resp.Body.Close()
	if !result.OK {
		t.Fatalf("result = %+v, want OK", result)
	}
	if v, _ := dst.Peek(0x0200); v != 0x42 {
		t.Fatalf("Peek($0200) = %02X, want 42", v)
	}
	if s.halted {
		t.Fatal("restore should clear the halted flag via reset")
	}
}

func TestSnapshotLoadRejectsWrongSize(t *testing.T) {
	srv := httptest.NewServer(NewServer(kim1.New()).Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/snapshot.bin", "application/octet-stream", bytes.NewReader(make([]byte, 100)))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	var result loadResultMsg
	json.NewDecoder(resp.Body).Decode(&result)
	if result.OK || result.Message == "" {
		t.Fatalf("result = %+v, want a non-OK result with a message", result)
	}
}
