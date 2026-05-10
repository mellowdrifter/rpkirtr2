# rpkirtr2

[![Go Report Card](https://goreportcard.com/badge/github.com/mellowdrifter/rpkirtr2)](https://goreportcard.com/report/github.com/mellowdrifter/rpkirtr2)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

An advanced, high-performance RPKI-to-Router (RTR) server written in Go, fully implementing RTR Version 2 (`draft-ietf-sidrops-8210bis`).

`rpkirtr2` is built to seamlessly deliver Resource Public Key Infrastructure (RPKI) validated prefix origin data, router keys, and ASPA to BGP daemons for Route Origin Validation (ROV). Designed for modern network architectures, it efficiently handles full internet routing tables while maintaining a strict, minimal resource footprint.

## Key Features

- **Full RTR v2 Support:** Implements the latest protocol draft including ASPA (Autonomous System Provider Authorization) payloads, while maintaining backward compatibility for RFC 8210 (v1) and RFC 6810 (v0).
- **High Performance & Memory Optimized:** Leverages Go's `sync.Pool` for highly efficient byte buffer management. Custom struct keys and stack buffers minimize garbage collection overhead.
- **Graceful Restarts:** Native support for zero-downtime reloads. The daemon can refresh its internal cache and reload configurations without dropping established TCP sessions with BGP speakers.
- **Dual-Stack:** Complete IPv4 and IPv6 support for all client connections.
- **VRP Expiry:** Handles VRP (Validated ROA Payload) expiry by evaluating validity timebounds on incoming datasets and removing expired ROAs.
- **gRPC Stats API:** Exposes cache state, refresh metrics, and serial number information via a gRPC interface.

## Architecture

`rpkirtr2` is designed around a multi-tier caching architecture to prevent concurrent map writes and provide zero-downtime reads to downstream BGP routers:

1. **Updater Routine:** Periodically fetches ROA, ASPA, and Router Key data from upstream RPKI JSON endpoints.
2. **Double-Buffering Cache:** A background "staging" cache is constructed and diffed against the "active" cache. This lock-free read path enables clients to pull updates concurrently without blocking the cache builder.
3. **PDU Marshaller:** Protocol Data Units are serialized using an optimized custom marshaling library employing fixed-size stack buffers to prevent heap-escapes during massive payload transfers.

## Memory Management

To support environments with limited resources, `rpkirtr2` incorporates advanced memory management techniques:

- **In-Place Deduplication:** ASPA and ROA arrays are deduplicated in-place, filtering out overlapping or duplicate prefixes without allocating extra slices.
- **Stack-Allocated Struct Keys:** The cache diffing mechanism uses struct-based keys rather than strings. This avoids the cost of string concatenation and subsequent string garbage collection.
- **Streaming JSON Decoding:** Custom JSON unmarshalers decode values directly into primitive types without allocating intermediate `interface{}`/`map[string]interface{}` boxing.

## Quick Start

### Build from Source

Requires [Go](https://go.dev/doc/install) 1.21 or later.

```bash
git clone https://github.com/mellowdrifter/rpkirtr2.git
cd rpkirtr2
make build
# or run `go build -o bin/rpkirtr2 ./cmd/rpkirtr2`
```

### Configuration

`rpkirtr2` is configured via a YAML file (`config.yaml`). Example configuration:

```yaml
listen_addr: ":8282"       # Address to listen on for RPKI-RTR clients
grpc_addr: ":50051"        # Address to listen on for gRPC statistics
log_level: "info"          # Log level (debug, info, warn, error)
rpki_urls:                 # List of RPKI JSON URLs to fetch data from
  - "https://rpki.gin.ntt.net/api/export.json"
  - "https://console.rpki-client.org/vrps.json"
```

To run the server:

```bash
./bin/rpkirtr2 -config config.yaml
```

## gRPC Statistics API

`rpkirtr2` exposes server statistics over gRPC. You can use standard tools like `grpcurl` or the provided Go client library to query:
- Last successful fetch time
- Number of active ROAs, ASPAs, and Router Keys
- Current cache serial number
- Uptime and memory footprint

## Testing

The project contains a comprehensive test suite including:
- Protocol unit tests for all PDU types
- End-to-End client handler integration tests
- Fuzz testing against protocol parsers to ensure memory safety on malicious input

Run the test suite with:

```bash
make test
```