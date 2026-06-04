package main

import (
	"bytes"
	"errors"
	"io"

	"github.com/siderolabs/talos/pkg/machinery/resources/network"
	"go.yaml.in/yaml/v4"
)

// validateConfig strictly decodes data as the metal platform network
// configuration (the format Talos reads from META key 0x0a), rejecting
// unknown fields, malformed values and extra YAML documents.
//
// See https://docs.siderolabs.com/talos/v1.13/platform-specific-installations/bare-metal-platforms/metal-network-configuration.
func validateConfig(data []byte) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)

	var cfg network.PlatformConfigSpec

	if err := dec.Decode(&cfg); err != nil {
		if errors.Is(err, io.EOF) {
			return errors.New("configuration is empty")
		}

		return err
	}

	if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
		return errors.New("unexpected extra YAML document")
	}

	return nil
}
