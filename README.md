# Talos metadata writer tool

Tool for writing network metadata in the Talos META partition.

Doc: https://docs.siderolabs.com/talos/v1.13/platform-specific-installations/bare-metal-platforms/metal-network-configuration


Compile:
```
GOOS=linux GOARCH=amd64 go build -o talos-meta-tool .
```

Usage:
```bash
# write config from file
talos-meta-tool -device /dev/sda -config config.yaml

# config from environment variable (plain text)
talos-meta-tool -device /dev/sda -config-env MY_CONFIG

# config from environment variable (base64-encoded, safe for multi-line YAML)
talos-meta-tool -device /dev/sda -config-env MY_CONFIG -config-env-base64
```

The `-device` flag accepts the full disk (e.g. `/dev/sda`); the META partition is discovered automatically via GPT.

The `-config` file is validated against the Talos v1.13 metal network configuration schema (`network.PlatformConfigSpec`) before writing: unknown fields, malformed addresses and invalid enum values are rejected.

To bypass validation — e.g. when the config uses fields that a newer Talos version has added but this tool's schema does not yet know about — pass `-skip-validation`.

Docker image:
```bash
# Linux (GNU base64)
docker run --rm --privileged -e MY_CONFIG="$(base64 -w0 config.yaml)" \
  ghcr.io/cozystack/talos-meta-tool:latest \
  -device /dev/sda -config-env MY_CONFIG -config-env-base64

# macOS/BSD
docker run --rm --privileged -e MY_CONFIG="$(base64 config.yaml | tr -d '\n')" \
  ghcr.io/cozystack/talos-meta-tool:latest \
  -device /dev/sda -config-env MY_CONFIG -config-env-base64
```
