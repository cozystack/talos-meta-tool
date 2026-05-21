FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /talos-meta-tool .

FROM scratch
COPY --from=builder /talos-meta-tool /talos-meta-tool
ENTRYPOINT ["/talos-meta-tool"]
