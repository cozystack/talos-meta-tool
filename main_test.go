package main

import (
	"bytes"
	"os"
	"testing"

	"github.com/siderolabs/go-adv/adv/talos"
)

// newTestFile creates a zeroed temp file of talos.Size bytes (two ADV blocks).
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

	if err := f.Truncate(talos.Size); err != nil {
		t.Fatal(err)
	}

	return f
}

func TestValidateYAMLValid(t *testing.T) {
	out, err := validateYAML([]byte("key: value\nfoo: bar\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Output must itself be valid and stable (idempotent).
	out2, err := validateYAML(out)
	if err != nil {
		t.Fatalf("output is not valid YAML: %v", err)
	}

	if !bytes.Equal(out, out2) {
		t.Fatalf("validateYAML not idempotent:\n first: %q\nsecond: %q", out, out2)
	}
}

func TestValidateYAMLInvalid(t *testing.T) {
	if _, err := validateYAML([]byte("key: [\ninvalid")); err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

func TestWriteConfigRoundTrip(t *testing.T) {
	f := newTestFile(t)

	payload := []byte("key: value\n")

	if err := writeConfig(f.Name(), payload); err != nil {
		t.Fatalf("writeConfig: %v", err)
	}

	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}

	loaded, err := talos.NewADV(f)
	if err != nil {
		t.Fatalf("NewADV: %v", err)
	}

	got, ok := loaded.ReadTagBytes(FixedTag)
	if !ok {
		t.Fatalf("tag %#x not found after round-trip", FixedTag)
	}

	if !bytes.Equal(got, payload) {
		t.Fatalf("tag value: got %q, want %q", got, payload)
	}
}

func TestWriteConfigPreservesExistingTags(t *testing.T) {
	f := newTestFile(t)

	// Write an initial tag using a different tag value.
	adv, err := talos.NewADV(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !adv.SetTagBytes(0x01, []byte("existing")) {
		t.Fatal("SetTagBytes 0x01 failed")
	}
	data, err := adv.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	if _, err := f.WriteAt(data, 0); err != nil {
		t.Fatal(err)
	}

	// writeConfig should preserve tag 0x01 while adding FixedTag.
	if err := writeConfig(f.Name(), []byte("key: value\n")); err != nil {
		t.Fatalf("writeConfig: %v", err)
	}

	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}

	loaded, err := talos.NewADV(f)
	if err != nil {
		t.Fatalf("NewADV: %v", err)
	}

	if _, ok := loaded.ReadTagBytes(0x01); !ok {
		t.Error("pre-existing tag 0x01 was lost after writeConfig")
	}

	if _, ok := loaded.ReadTagBytes(FixedTag); !ok {
		t.Errorf("tag %#x not found after writeConfig", FixedTag)
	}
}

func TestWriteConfigOversizedPayload(t *testing.T) {
	f := newTestFile(t)

	if err := writeConfig(f.Name(), make([]byte, talos.DataLength+1)); err == nil {
		t.Fatal("expected error for oversized payload, got nil")
	}
}

func TestWriteConfigReadOnlyDevice(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("read-only permission check is ineffective as root")
	}

	f := newTestFile(t)

	if err := os.Chmod(f.Name(), 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(f.Name(), 0o644) }) //nolint:errcheck

	if err := writeConfig(f.Name(), []byte("key: value\n")); err == nil {
		t.Fatal("expected error writing to read-only device, got nil")
	}
}

func TestWriteConfigBadDevice(t *testing.T) {
	if err := writeConfig("/nonexistent/path/device", []byte("key: value\n")); err == nil {
		t.Fatal("expected error for non-existent device, got nil")
	}
}
