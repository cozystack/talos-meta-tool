package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"sync"

	"gopkg.in/yaml.v3"
)

const (
	FixedTag   = 0xA         // Fixed tag
	Magic1     = 0x5a4b3c2d  // Magic value 1
	Magic2     = 0xa5b4c3d2  // Magic value 2
	Length     = 256 * 1024  // ADV size in bytes
	DataLength = Length - 40 // Available space for data
)

type ADV struct {
	Tags map[uint8][]byte
	mu   sync.Mutex
}

// NewADV initializes ADV. If the device does not contain a valid Magic1, an empty ADV is returned.
func NewADV(r io.Reader) (*ADV, error) {
	a := &ADV{
		Tags: make(map[uint8][]byte),
	}

	if r == nil {
		return a, nil
	}

	buf := make([]byte, Length)
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return nil, err
	}

	if err = a.unmarshal(buf); err != nil {
		log.Printf("ADV does not contain a valid Magic1: initializing a new ADV.")
		return &ADV{Tags: make(map[uint8][]byte)}, nil
	}

	return a, nil
}

// unmarshal loads data from the buffer into the ADV structure
func (a *ADV) unmarshal(buf []byte) error {
	magic1 := binary.BigEndian.Uint32(buf[:4])
	if magic1 != Magic1 {
		return fmt.Errorf("adv: incorrect magic1 value: %x", magic1)
	}

	magic2 := binary.BigEndian.Uint32(buf[len(buf)-4:])
	if magic2 != Magic2 {
		return fmt.Errorf("adv: incorrect magic2 value: %x", magic2)
	}

	checksum := append([]byte(nil), buf[len(buf)-36:len(buf)-4]...)
	copy(buf[len(buf)-36:len(buf)-4], make([]byte, 32))

	hash := sha256.Sum256(buf)
	if !bytes.Equal(checksum, hash[:]) {
		return fmt.Errorf("adv: invalid checksum")
	}

	data := buf[4 : len(buf)-36]
	for len(data) >= 8 {
		tag := data[0]
		if tag == 0 {
			break
		}

		size := binary.BigEndian.Uint32(data[4:8])
		if len(data) < int(size)+8 {
			return fmt.Errorf("adv: value exceeds buffer limits")
		}

		value := data[8 : 8+size]
		a.Tags[tag] = value
		data = data[8+size:]
	}

	return nil
}

// SetTagBytes sets the tag value in byte format
func (a *ADV) SetTagBytes(tag uint8, val []byte) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	size := 20 // magic and checksum
	for _, v := range a.Tags {
		size += len(v) + 8
	}

	if len(val)+size > DataLength {
		return false
	}

	a.Tags[tag] = val
	return true
}

// marshal converts ADV data into a byte array
func (a *ADV) marshal() ([]byte, error) {
	buf := make([]byte, Length)
	binary.BigEndian.PutUint32(buf[0:4], Magic1)
	binary.BigEndian.PutUint32(buf[len(buf)-4:], Magic2)

	data := buf[4 : len(buf)-36]
	for tag, value := range a.Tags {
		data[0] = tag
		binary.BigEndian.PutUint32(data[4:8], uint32(len(value)))
		copy(data[8:8+len(value)], value)
		data = data[8+len(value):]
	}

	hash := sha256.Sum256(buf)
	copy(buf[len(buf)-36:len(buf)-4], hash[:])
	return buf, nil
}

// WriteToDisk writes ADV data to disk
func (a *ADV) WriteToDisk(devicePath string) error {
	serialized, err := a.marshal()
	if err != nil {
		return err
	}

	f, err := os.OpenFile(devicePath, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteAt(serialized, 0)
	if err != nil {
		return err
	}

	_, err = f.Seek(0, io.SeekStart)
	if err != nil {
		return err
	}

	_, err = f.WriteAt(serialized, Length)
	return err
}

func main() {
	// Command-line arguments
	devicePath := flag.String("device", "", "Path to the META device (e.g., /dev/sda4)")
	configPath := flag.String("config", "", "Path to the configuration file (e.g., config.yaml)")
	flag.Parse()

	if *devicePath == "" || *configPath == "" {
		fmt.Println("Usage: go run main.go -device <META-device> -config <path to config file>")
		return
	}

	// Reading configuration from file
	configData, err := ioutil.ReadFile(*configPath)
	if err != nil {
		log.Fatalf("Error reading configuration file: %v", err)
	}

	// YAML validation
	var config interface{}
	if err := yaml.Unmarshal(configData, &config); err != nil {
		log.Fatalf("Invalid YAML configuration: %v", err)
	}

	// Marshaling back to ensure correct format
	validatedConfigData, err := yaml.Marshal(config)
	if err != nil {
		log.Fatalf("Error marshaling YAML configuration: %v", err)
	}

	// Creating or loading an existing ADV
	var adv *ADV
	f, err := os.Open(*devicePath)
	if err == nil {
		defer f.Close()
		adv, err = NewADV(f)
		if err != nil {
			log.Fatalf("Error loading ADV: %v", err)
		}
	} else {
		adv = &ADV{
			Tags: make(map[uint8][]byte),
		}
	}

	// Writing validated configuration into ADV
	if !adv.SetTagBytes(FixedTag, validatedConfigData) {
		log.Fatalf("Error: not enough space to write configuration")
	}

	// Writing data to disk
	if err := adv.WriteToDisk(*devicePath); err != nil {
		log.Fatalf("Error writing data to disk: %v", err)
	}

	fmt.Println("Configuration successfully validated and written to META partition.")
}
