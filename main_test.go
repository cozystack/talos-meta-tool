package main

import (
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

	t.Cleanup(func() { os.Remove(f.Name()) })

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
	defer f.Close()

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

func TestWriteToDiskWritesBothCopies(t *testing.T) {
	f := newTestFile(t)
	defer f.Close()

	adv := &ADV{Tags: make(map[uint8][]byte)}
	adv.SetTagBytes(FixedTag, []byte("test"))

	if err := adv.WriteToDisk(f.Name()); err != nil {
		t.Fatalf("WriteToDisk: %v", err)
	}

	magic := make([]byte, 4)

	// first copy at offset 0
	if _, err := f.ReadAt(magic, 0); err != nil {
		t.Fatal(err)
	}

	if got := binary.BigEndian.Uint32(magic); got != Magic1 {
		t.Fatalf("first copy Magic1: got %#x, want %#x", got, Magic1)
	}

	// second copy at offset Length
	if _, err := f.ReadAt(magic, Length); err != nil {
		t.Fatal(err)
	}

	if got := binary.BigEndian.Uint32(magic); got != Magic1 {
		t.Fatalf("second copy Magic1: got %#x, want %#x", got, Magic1)
	}
}
