package main

import (
	"bytes"
	"encoding/binary"
	"os"
	"testing"
)

// newTestFile creates a zeroed temp file of 2*Length bytes (two ADV blocks).
func newTestFile(t *testing.T) *os.File {
	t.Helper()

	f, err := os.CreateTemp("", "adv-*.bin")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if err := f.Close(); err != nil {
			t.Errorf("close temp file: %v", err)
		}

		if err := os.Remove(f.Name()); err != nil && !os.IsNotExist(err) {
			t.Errorf("remove temp file: %v", err)
		}
	})

	if err := f.Truncate(2 * Length); err != nil {
		t.Fatal(err)
	}

	return f
}

func TestNewADVNilReader(t *testing.T) {
	adv, err := NewADV(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(adv.Tags) != 0 {
		t.Fatalf("expected empty Tags, got %d entries", len(adv.Tags))
	}
}

func TestNewADVEmptyFile(t *testing.T) {
	f := newTestFile(t)

	// A zeroed file has no valid magic — NewADV should return an empty ADV, not an error.
	adv, err := NewADV(f)
	if err != nil {
		t.Fatalf("unexpected error on zeroed file: %v", err)
	}

	if len(adv.Tags) != 0 {
		t.Fatalf("expected empty Tags, got %d entries", len(adv.Tags))
	}
}

func TestSetTagBytes(t *testing.T) {
	adv := &ADV{Tags: make(map[uint8][]byte)}

	val := []byte("hello")
	if !adv.SetTagBytes(FixedTag, val) {
		t.Fatal("SetTagBytes returned false unexpectedly")
	}

	got, ok := adv.Tags[FixedTag]
	if !ok {
		t.Fatal("tag not found in map after SetTagBytes")
	}

	if string(got) != string(val) {
		t.Fatalf("got %q, want %q", got, val)
	}
}

func TestSetTagBytesOverflow(t *testing.T) {
	adv := &ADV{Tags: make(map[uint8][]byte)}

	oversized := make([]byte, DataLength+1)
	if adv.SetTagBytes(FixedTag, oversized) {
		t.Fatal("SetTagBytes should return false for oversized value")
	}
}

func TestMarshalMagicBytes(t *testing.T) {
	adv := &ADV{Tags: make(map[uint8][]byte)}
	adv.SetTagBytes(FixedTag, []byte("test"))

	buf, err := adv.marshal()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if len(buf) != Length {
		t.Fatalf("marshal output length: got %d, want %d", len(buf), Length)
	}

	if got := binary.BigEndian.Uint32(buf[:4]); got != Magic1 {
		t.Fatalf("Magic1: got %#x, want %#x", got, Magic1)
	}

	if got := binary.BigEndian.Uint32(buf[len(buf)-4:]); got != Magic2 {
		t.Fatalf("Magic2: got %#x, want %#x", got, Magic2)
	}
}

func TestRoundTripEmpty(t *testing.T) {
	original := &ADV{Tags: make(map[uint8][]byte)}

	buf, err := original.marshal()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	loaded := &ADV{Tags: make(map[uint8][]byte)}
	if err := loaded.unmarshal(buf); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(loaded.Tags) != 0 {
		t.Fatalf("expected empty Tags after round-trip, got %d entries", len(loaded.Tags))
	}
}

func TestRoundTripWithTags(t *testing.T) {
	original := &ADV{Tags: make(map[uint8][]byte)}
	original.SetTagBytes(FixedTag, []byte("hello world"))

	buf, err := original.marshal()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	loaded := &ADV{Tags: make(map[uint8][]byte)}
	if err := loaded.unmarshal(buf); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	got, ok := loaded.Tags[FixedTag]
	if !ok {
		t.Fatalf("tag %#x not found after round-trip; got tags: %v", FixedTag, loaded.Tags)
	}

	if string(got) != "hello world" {
		t.Fatalf("tag value: got %q, want %q", got, "hello world")
	}
}

func TestMarshalTagLayout(t *testing.T) {
	adv := &ADV{Tags: make(map[uint8][]byte)}
	adv.SetTagBytes(FixedTag, []byte("xyz"))

	buf, err := adv.marshal()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Tag header sits right after Magic1 (4 bytes): tag (1B) + 3B reserved + size (4B BE).
	if got := buf[4]; got != FixedTag {
		t.Fatalf("on-wire tag byte: got %#x, want %#x", got, FixedTag)
	}

	if got := binary.BigEndian.Uint32(buf[8:12]); got != 3 {
		t.Fatalf("on-wire size: got %d, want 3", got)
	}

	if got := string(buf[12:15]); got != "xyz" {
		t.Fatalf("on-wire value: got %q, want %q", got, "xyz")
	}
}

func TestDiskRoundTrip(t *testing.T) {
	f := newTestFile(t)

	original := &ADV{Tags: make(map[uint8][]byte)}
	original.SetTagBytes(FixedTag, []byte("disk round-trip"))

	if err := original.WriteToDisk(f.Name()); err != nil {
		t.Fatalf("WriteToDisk: %v", err)
	}

	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}

	loaded, err := NewADV(f)
	if err != nil {
		t.Fatalf("NewADV: %v", err)
	}

	got, ok := loaded.Tags[FixedTag]
	if !ok {
		t.Fatalf("tag %#x not found after disk round-trip; got tags: %v", FixedTag, loaded.Tags)
	}

	if string(got) != "disk round-trip" {
		t.Fatalf("tag value: got %q, want %q", got, "disk round-trip")
	}
}

func TestWriteToDiskWritesBothCopies(t *testing.T) {
	f := newTestFile(t)

	adv := &ADV{Tags: make(map[uint8][]byte)}
	adv.SetTagBytes(FixedTag, []byte("test"))

	if err := adv.WriteToDisk(f.Name()); err != nil {
		t.Fatalf("WriteToDisk: %v", err)
	}

	buf1 := make([]byte, Length)
	buf2 := make([]byte, Length)

	if _, err := f.ReadAt(buf1, 0); err != nil {
		t.Fatal(err)
	}

	if _, err := f.ReadAt(buf2, Length); err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(buf1, buf2) {
		t.Error("second copy does not match first copy")
	}

	if got := binary.BigEndian.Uint32(buf1[:4]); got != Magic1 {
		t.Fatalf("Magic1: got %#x, want %#x", got, Magic1)
	}
}
