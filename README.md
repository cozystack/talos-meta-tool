# Talos metadata writer tool

Tool for writing network metadata in the Talos META partition.

Doc: https://docs.siderolabs.com/talos/v1.13/platform-specific-installations/bare-metal-platforms/metal-network-configuration


Compile:
```
GOOS=linux GOARCH=amd64 go build -o talos-meta-tool .
```

Usage:
```
talos-meta-tool -device /dev/sda -config config.yaml
```

The `-device` flag accepts the full disk (e.g. `/dev/sda`); the META partition is discovered automatically via GPT.

The `-config` file is validated against the Talos v1.13 metal network configuration schema (`network.PlatformConfigSpec`) before writing: unknown fields, malformed addresses and invalid enum values are rejected.

To bypass validation — e.g. when the config uses fields that a newer Talos version has added but this tool's schema does not yet know about — pass `-skip-validation`.
