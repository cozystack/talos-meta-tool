# Talos metadata writer tool

Tool for writing network metadata in the Talos META partition.

Doc: https://www.talos.dev/v1.8/advanced/metal-network-configuration/


Compile:
```
GOOS=linux GOARCH=amd64 go build -o talos-meta-tool .
```

Usage:
```
talos-meta-tool -device /dev/sda -config config.yaml
```

The `-device` flag accepts the full disk (e.g. `/dev/sda`); the META partition is discovered automatically via GPT.

The `-config` file is validated against the Talos metal network configuration schema (`network.PlatformConfigSpec`) before writing: unknown fields, malformed addresses and invalid enum values are rejected.
