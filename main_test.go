//go:build linux

package main

import (
	"bytes"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/siderolabs/go-adv/adv/talos"
	"github.com/siderolabs/go-blockdevice/v2/partitioning/gpt"
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

const (
	mib      = 1024 * 1024
	diskSize = 20 * mib
)

// linuxFSType is the GPT type GUID for Linux filesystem data.
var linuxFSType = uuid.MustParse("0FC63DAF-8483-4772-8E79-3D69D8477DE4")

// newTestDisk creates a disk image with a Talos-like GPT layout.
func newTestDisk(t *testing.T) *os.File {
	t.Helper()

	f, err := os.CreateTemp("", "disk-*.img")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		f.Close() //nolint:errcheck
		if err := os.Remove(f.Name()); err != nil && !os.IsNotExist(err) {
			t.Errorf("remove temp disk: %v", err)
		}
	})

	if err := f.Truncate(diskSize); err != nil {
		t.Fatal(err)
	}

	gptdev, err := gpt.DeviceFromFile(f)
	if err != nil {
		t.Fatal(err)
	}

	table, err := gpt.New(gptdev)
	if err != nil {
		t.Fatal(err)
	}

	efiType := uuid.MustParse("C12A7328-F81F-11D2-BA4B-00A0C93EC93B")
	for _, part := range []struct {
		name string
		typ  uuid.UUID
	}{
		{"EFI", efiType},
		{"BIOS", linuxFSType},
		{"BOOT", linuxFSType},
		{"META", linuxFSType},
		{"STATE", linuxFSType},
		{"EPHEMERAL", linuxFSType},
	} {
		if _, _, err := table.AllocatePartition(1*mib, part.name, part.typ); err != nil {
			t.Fatalf("AllocatePartition %s: %v", part.name, err)
		}
	}

	if err := table.Write(); err != nil {
		t.Fatal(err)
	}

	return f
}

// errDevice always returns an error for both ReadAt and WriteAt.
type errDevice struct{}

func (errDevice) ReadAt(p []byte, off int64) (int, error) {
	return 0, errors.New("device error")
}

func (errDevice) WriteAt(p []byte, off int64) (int, error) {
	return 0, errors.New("device error")
}

func TestWriteConfigRoundTrip(t *testing.T) {
	f := newTestFile(t)

	payload := []byte("key: value\n")

	if err := writeConfig(f, payload); err != nil {
		t.Fatalf("writeConfig: %v", err)
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
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
	if err := writeConfig(f, []byte("key: value\n")); err != nil {
		t.Fatalf("writeConfig: %v", err)
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
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

	if err := writeConfig(f, make([]byte, talos.DataLength+1)); err == nil {
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

	ro, err := os.Open(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close() //nolint:errcheck

	if err := writeConfig(ro, []byte("key: value\n")); err == nil {
		t.Fatal("expected error writing to read-only device, got nil")
	}
}

func TestWriteConfigBadDevice(t *testing.T) {
	if err := writeConfig(errDevice{}, []byte("key: value\n")); err == nil {
		t.Fatal("expected error for bad device, got nil")
	}
}

func TestFindMetaPartitionNoGPT(t *testing.T) {
	f, err := os.CreateTemp("", "nogpt-*.img")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		f.Close()       //nolint:errcheck
		os.Remove(f.Name()) //nolint:errcheck
	})
	if err := f.Truncate(diskSize); err != nil {
		t.Fatal(err)
	}

	if _, err := findMetaPartition(f); err == nil {
		t.Fatal("expected error for disk with no GPT, got nil")
	}
}

func TestFindMetaPartitionMissing(t *testing.T) {
	f, err := os.CreateTemp("", "nometa-*.img")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		f.Close()       //nolint:errcheck
		os.Remove(f.Name()) //nolint:errcheck
	})
	if err := f.Truncate(diskSize); err != nil {
		t.Fatal(err)
	}

	gptdev, err := gpt.DeviceFromFile(f)
	if err != nil {
		t.Fatal(err)
	}
	table, err := gpt.New(gptdev)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := table.AllocatePartition(1*mib, "STATE", linuxFSType); err != nil {
		t.Fatal(err)
	}
	if err := table.Write(); err != nil {
		t.Fatal(err)
	}

	if _, err := findMetaPartition(f); err == nil {
		t.Fatal("expected error when META partition is absent, got nil")
	}
}

func TestLoadConfigFile(t *testing.T) {
	f, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(f.Name()) }) //nolint:errcheck

	content := []byte("key: value\n")
	if _, err := f.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	got, err := loadConfig(f.Name(), "", false)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("got %q, want %q", got, content)
	}
}

func TestLoadConfigFileMissing(t *testing.T) {
	if _, err := loadConfig("/nonexistent/config.yaml", "", false); err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoadConfigEnv(t *testing.T) {
	content := "key: value\n"
	t.Setenv("TEST_META_CONFIG", content)

	got, err := loadConfig("", "TEST_META_CONFIG", false)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if string(got) != content {
		t.Fatalf("got %q, want %q", got, content)
	}
}

func TestLoadConfigEnvNotSet(t *testing.T) {
	if _, err := loadConfig("", "TEST_META_CONFIG_UNSET_XYZ", false); err == nil {
		t.Fatal("expected error for unset env var, got nil")
	}
}

func TestLoadConfigEnvBase64(t *testing.T) {
	content := []byte("key: value\n")
	encoded := base64.StdEncoding.EncodeToString(content)

	tests := []struct {
		name  string
		input string
	}{
		{"no whitespace", encoded},
		{"trailing newline", encoded + "\n"},
		{"embedded newlines", encoded[:len(encoded)/2] + "\n" + encoded[len(encoded)/2:]},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TEST_META_CONFIG_B64", tc.input)
			got, err := loadConfig("", "TEST_META_CONFIG_B64", true)
			if err != nil {
				t.Fatalf("loadConfig: %v", err)
			}
			if !bytes.Equal(got, content) {
				t.Fatalf("got %q, want %q", got, content)
			}
		})
	}
}

func TestLoadConfigEnvBase64Invalid(t *testing.T) {
	t.Setenv("TEST_META_CONFIG_B64", "not-valid-base64!!!")
	if _, err := loadConfig("", "TEST_META_CONFIG_B64", true); err == nil {
		t.Fatal("expected error for invalid base64, got nil")
	}
}

func TestWriteConfigFullDisk(t *testing.T) {
	diskFile := newTestDisk(t)

	payload := []byte("hostname: talos-test\n")

	meta, err := findMetaPartition(diskFile)
	if err != nil {
		t.Fatalf("findMetaPartition: %v", err)
	}

	if err := writeConfig(meta, payload); err != nil {
		t.Fatalf("writeConfig: %v", err)
	}

	// Read back via GPT to verify the data landed in the right partition.
	meta2, err := findMetaPartition(diskFile)
	if err != nil {
		t.Fatalf("findMetaPartition (read-back): %v", err)
	}

	loaded, err := talos.NewADV(io.NewSectionReader(meta2, 0, int64(talos.Size)))
	if err != nil {
		t.Fatalf("NewADV: %v", err)
	}

	got, ok := loaded.ReadTagBytes(FixedTag)
	if !ok {
		t.Fatalf("tag %#x not found in META partition after full-disk write", FixedTag)
	}

	if !bytes.Equal(got, payload) {
		t.Fatalf("tag value: got %q, want %q", got, payload)
	}
}

func TestReadConfigRoundTrip(t *testing.T) {
	f := newTestFile(t)

	payload := []byte("key: value\n")

	if err := writeConfig(f, payload); err != nil {
		t.Fatalf("writeConfig: %v", err)
	}

	got, err := readConfig(f)
	if err != nil {
		t.Fatalf("readConfig: %v", err)
	}

	if !bytes.Equal(got, payload) {
		t.Fatalf("readConfig: got %q, want %q", got, payload)
	}
}

func TestReadConfigEmpty(t *testing.T) {
	f := newTestFile(t)

	if _, err := readConfig(f); err == nil {
		t.Fatal("expected error reading from empty ADV, got nil")
	}
}

func TestReadConfigBadDevice(t *testing.T) {
	if _, err := readConfig(errDevice{}); err == nil {
		t.Fatal("expected error for bad device, got nil")
	}
}
