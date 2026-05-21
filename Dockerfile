FROM golang:1.26.3-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /talos-meta-tool .

FROM scratch
COPY --from=builder /talos-meta-tool /talos-meta-tool
ENTRYPOINT ["/talos-meta-tool"]
