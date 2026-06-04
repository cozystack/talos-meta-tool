package main

import (
	"strings"
	"testing"
)

// validNetworkConfig follows the metal network configuration format,
// see https://docs.siderolabs.com/talos/v1.13/platform-specific-installations/bare-metal-platforms/metal-network-configuration.
const validNetworkConfig = `addresses:
  - address: 147.75.61.43/31
    linkName: bond0
    family: inet4
    scope: global
    flags: permanent
    layer: platform
links:
  - name: eth0
    up: true
    layer: platform
  - name: bond0
    logical: true
    up: true
    mtu: 1500
    kind: bond
    type: ether
    bondMaster:
      mode: 802.3ad
      xmitHashPolicy: layer3+4
      lacpRate: fast
      miimon: 100
      updelay: 300
      downdelay: 200
    layer: platform
routes:
  - family: inet4
    gateway: 147.75.61.42
    outLinkName: bond0
    table: main
    scope: global
    type: unicast
    protocol: static
    layer: platform
hostnames:
  - hostname: ci-blue-worker-amd64-2
    layer: platform
resolvers:
  - dnsServers:
      - 8.8.8.8
      - 1.1.1.1
    layer: platform
timeServers:
  - timeServers:
      - pool.ntp.org
    layer: platform
operators:
  - operator: dhcp4
    linkName: eth1
    requireUp: true
    dhcp4:
      routeMetric: 1024
    layer: platform
externalIPs:
  - 147.75.61.43
`

func TestValidateConfigValid(t *testing.T) {
	if err := validateConfig([]byte(validNetworkConfig)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateConfigInvalid(t *testing.T) {
	for _, tt := range []struct {
		name   string
		config string
	}{
		{"malformed YAML", "key: [\ninvalid"},
		{"empty", ""},
		{"unknown top-level field", "hostname: talos-test\n"},
		{"unknown nested field", "addresses:\n  - adress: 1.2.3.4/32\n"},
		{"wrong type", "addresses: notalist\n"},
		{"bad enum value", "addresses:\n  - address: 1.2.3.4/32\n    family: inet5\n"},
		{"bad IP", "externalIPs:\n  - not-an-ip\n"},
		{"extra document", validNetworkConfig + "---\nexternalIPs: []\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateConfig([]byte(tt.config)); err == nil {
				t.Fatalf("expected error, got nil")
			}
		})
	}
}

func TestValidateConfigUnknownFieldError(t *testing.T) {
	err := validateConfig([]byte("addresses: []\nfoo: bar\n"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "foo") {
		t.Fatalf("error should mention the unknown field, got: %v", err)
	}
}
