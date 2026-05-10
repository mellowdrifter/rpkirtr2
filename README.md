# rpkirtr2

[![Go Report Card](https://goreportcard.com/badge/github.com/mellowdrifter/rpkirtr2)](https://goreportcard.com/report/github.com/mellowdrifter/rpkirtr2)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

An advanced, high-performance RPKI-to-Router (RTR) server written in Go, fully implementing RTR Version 2 (`draft-ietf-sidrops-8210bis`).

`rpkirtr2` is built to seamlessly deliver Resource Public Key Infrastructure (RPKI) validated prefix origin data and router keys to BGP daemons for Route Origin Validation (ROV). Designed for modern network architectures, it efficiently handles full internet routing tables while maintaining a strict, minimal resource footprint.

## Key Features

- **Full RTR v2 Support:** Implements the latest protocol draft including ASPA (Autonomous System Provider Authorization) payloads, while maintaining backward compatibility for RFC 8210 (v1) and RFC 6810 (v0).
- **High Performance & Memory Optimized:** Leverages Go's `sync.Pool` for highly efficient byte buffer management. This drastically reduces garbage collection pressure when marshaling hundreds of thousands of VRPs (Validated ROA Payloads) for connected peers.
- **Graceful Restarts:** Native support for zero-downtime reloads. The daemon can refresh its internal cache and reload configurations without dropping established TCP sessions with BGP speakers.
- **Dual-Stack:** Complete IPv4 and IPv6 support for all client connections.

## Quick Start

### Build from Source

Requires [Go](https://go.dev/doc/install) 1.21 or later.

```bash
git clone [https://github.com/mellowdrifter/rpkirtr2.git](https://github.com/mellowdrifter/rpkirtr2.git)
cd rpkirtr2
go build -o rpkirtr2