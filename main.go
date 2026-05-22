package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/siderolabs/go-adv/adv/talos"
	"gopkg.in/yaml.v3"
)

const FixedTag = 0xA // Fixed tag

func validateYAML(data []byte) ([]byte, error) {
	var config interface{}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	return yaml.Marshal(config)
}

func writeConfig(devicePath string, configData []byte) (err error) {
	var adv *talos.ADV
	f, err := os.Open(devicePath)
	if err == nil {
		var loadErr error
		adv, loadErr = talos.NewADV(f)
		if cerr := f.Close(); cerr != nil && loadErr == nil {
			loadErr = cerr
		}
		if adv == nil {
			// nil means an I/O error; non-nil with error means empty/corrupt device
			return fmt.Errorf("loading ADV: %w", loadErr)
		}
	} else {
		adv, _ = talos.NewADV(nil)
	}

	if !adv.SetTagBytes(FixedTag, configData) {
		return fmt.Errorf("not enough space to write configuration")
	}

	data, err := adv.Bytes()
	if err != nil {
		return fmt.Errorf("serializing ADV: %w", err)
	}

	device, err := os.OpenFile(devicePath, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("opening device for writing: %w", err)
	}
	defer func() {
		if cerr := device.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	if _, err := device.WriteAt(data, 0); err != nil {
		return fmt.Errorf("writing data to disk: %w", err)
	}

	return nil
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
	configData, err := os.ReadFile(*configPath)
	if err != nil {
		log.Fatalf("Error reading configuration file: %v", err)
	}

	validatedConfigData, err := validateYAML(configData)
	if err != nil {
		log.Fatalf("Invalid YAML configuration: %v", err)
	}

	if err := writeConfig(*devicePath, validatedConfigData); err != nil {
		log.Fatalf("Error: %v", err)
	}

	fmt.Println("Configuration successfully validated and written to META partition.")
}
