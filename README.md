# Talos metadata writer tool

Tool for writing network metadata into META partition.

Doc: https://www.talos.dev/v1.8/advanced/metal-network-configuration/


Compile:
```
GOOS=linux GOARCH=amd64 go build -o talos-meta-tool main.go
```

Usage:
```
/talos-meta-tool -config config.yaml -device /dev/sda4
```
