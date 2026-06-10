//go:build linux

package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/siderolabs/go-adv/adv/talos"
	"github.com/siderolabs/go-blockdevice/v2/block"
	"github.com/siderolabs/go-blockdevice/v2/partitioning/gpt"
)

const FixedTag = 0xA // Fixed tag

// partAt wraps an *os.File and translates offset-0 reads/writes to a fixed
// byte offset within the file — used to address a partition inside a disk.
type partAt struct {
	file   *os.File
	offset int64
}

func (p *partAt) ReadAt(buf []byte, off int64) (int, error) {
	return p.file.ReadAt(buf, p.offset+off)
}

func (p *partAt) WriteAt(buf []byte, off int64) (int, error) {
	return p.file.WriteAt(buf, p.offset+off)
}

// openGPTDevice returns a gpt.Device for f. It uses ioctl for real block
// devices and falls back to stat for regular files (e.g. in tests).
func openGPTDevice(f *os.File) (gpt.Device, error) {
	if dev, err := gpt.DeviceFromBlockDevice(block.NewFromFile(f)); err == nil {
		return dev, nil
	}
	return gpt.DeviceFromFile(f)
}

// findMetaPartition reads the GPT table from f and returns a ReadWriteAt
// scoped to the partition named "META".
func findMetaPartition(f *os.File) (interface{ io.ReaderAt; io.WriterAt }, error) {
	gptdev, err := openGPTDevice(f)
	if err != nil {
		return nil, fmt.Errorf("opening GPT device: %w", err)
	}

	table, err := gpt.Read(gptdev)
	if err != nil {
		return nil, fmt.Errorf("reading GPT table: %w", err)
	}

	sectorSize := int64(gptdev.GetSectorSize())

	for _, p := range table.Partitions() {
		if p != nil && p.Name == "META" {
			return &partAt{file: f, offset: int64(p.FirstLBA) * sectorSize}, nil
		}
	}

	return nil, fmt.Errorf("META partition not found")
}

func writeConfig(dev interface{ io.ReaderAt; io.WriterAt }, configData []byte) error {
	adv, loadErr := talos.NewADV(io.NewSectionReader(dev, 0, int64(talos.Size)))
	if adv == nil {
		// nil means an I/O error; non-nil with error means empty/corrupt device
		return fmt.Errorf("loading ADV: %w", loadErr)
	}

	if !adv.SetTagBytes(FixedTag, configData) {
		return fmt.Errorf("not enough space to write configuration")
	}

	data, err := adv.Bytes()
	if err != nil {
		return fmt.Errorf("serializing ADV: %w", err)
	}

	if _, err = dev.WriteAt(data, 0); err != nil {
		return fmt.Errorf("writing data to disk: %w", err)
	}

	return nil
}

func loadConfig(path, envVar string, b64 bool) ([]byte, error) {
	if envVar != "" {
		val, ok := os.LookupEnv(envVar)
		if !ok {
			return nil, fmt.Errorf("environment variable %q is not set", envVar)
		}
		if b64 {
			stripped := strings.Map(func(r rune) rune {
				if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
					return -1
				}
				return r
			}, val)
			data, err := base64.StdEncoding.DecodeString(stripped)
			if err != nil {
				return nil, fmt.Errorf("base64 decoding %q: %w", envVar, err)
			}
			return data, nil
		}
		return []byte(val), nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading configuration file: %w", err)
	}
	return data, nil
}

func main() {
	devicePath := flag.String("device", "", "Path to the disk device (e.g., /dev/sda)")
	configPath := flag.String("config", "", "Path to the configuration file (e.g., config.yaml)")
	configEnv := flag.String("config-env", "", "Name of the environment variable containing the configuration")
	configEnvBase64 := flag.Bool("config-env-base64", false, "Decode the -config-env value as base64 before use")
	skipValidation := flag.Bool("skip-validation", false, "Skip schema validation of the configuration file")
	flag.Parse()

	if *devicePath == "" {
		fmt.Fprintln(os.Stderr, "Usage: talos-meta-tool -device <disk-device> (-config <file> | -config-env <VAR> [-config-env-base64])")
		os.Exit(1)
	}
	if *configPath == "" && *configEnv == "" {
		fmt.Fprintln(os.Stderr, "Usage: talos-meta-tool -device <disk-device> (-config <file> | -config-env <VAR> [-config-env-base64])")
		os.Exit(1)
	}
	if *configPath != "" && *configEnv != "" {
		fmt.Fprintln(os.Stderr, "Error: -config and -config-env are mutually exclusive")
		os.Exit(1)
	}
	if *configEnvBase64 && *configEnv == "" {
		fmt.Fprintln(os.Stderr, "Error: -config-env-base64 requires -config-env")
		os.Exit(1)
	}

	device, err := os.OpenFile(*devicePath, os.O_RDWR, 0)
	if err != nil {
		log.Fatalf("Error opening device: %v", err)
	}
	defer device.Close() //nolint:errcheck

	meta, err := findMetaPartition(device)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	configData, err := loadConfig(*configPath, *configEnv, *configEnvBase64)
	if err != nil {
		log.Fatalf("loading configuration: %v", err)
	}

	if !*skipValidation {
		if err := validateConfig(configData); err != nil {
			log.Fatalf("Invalid network configuration: %v", err)
		}
	}

	if err := writeConfig(meta, configData); err != nil {
		log.Fatalf("Error: %v", err)
	}

	if err := device.Sync(); err != nil {
		log.Fatalf("Error syncing device: %v", err)
	}

	fmt.Println("Configuration successfully validated and written to META partition.")
}
