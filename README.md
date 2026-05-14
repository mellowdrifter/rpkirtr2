# rpkirtr2

[![Go Report Card](https://goreportcard.com/badge/github.com/mellowdrifter/rpkirtr2)](https://goreportcard.com/report/github.com/mellowdrifter/rpkirtr2)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)
![Version](https://img.shields.io/badge/version-1.1.0-green.svg)

A high-performance, production-grade RPKI-to-Router (RTR) server written in Go. Implements RTR Version 2 ([draft-ietf-sidrops-8210bis](https://datatracker.ietf.org/doc/draft-ietf-sidrops-8210bis/)) with full backward compatibility for RTR Version 1 ([RFC 8210](https://www.rfc-editor.org/rfc/rfc8210)) and RTR Version 0 ([RFC 6810](https://www.rfc-editor.org/rfc/rfc6810)).

`rpkirtr2` fetches RPKI-validated data from upstream JSON feeds and serves it to BGP daemons and routers via the RTR protocol, enabling Route Origin Validation (ROV) and Autonomous System Provider Authorization (ASPA) policy enforcement.

---

## Contents

- [Protocol Support](#protocol-support)
- [Features](#features)
- [Architecture](#architecture)
- [Getting Started](#getting-started)
- [Configuration](#configuration)
- [Running in Production](#running-in-production)
- [gRPC Statistics API](#grpc-statistics-api)
- [Memory Management](#memory-management)
- [VRP Expiry](#vrp-expiry)
- [Client Behaviour](#client-behaviour)
- [Testing](#testing)

---

## Protocol Support

| Feature | Support |
|---|---|
| RTR Version 0 (RFC 6810) | ✅ |
| RTR Version 1 (RFC 8210) | ✅ |
| RTR Version 2 (draft-ietf-sidrops-8210bis) | ✅ |
| IPv4 Prefix PDU | ✅ |
| IPv6 Prefix PDU | ✅ |
| ASPA PDU (Type 11) | ✅ |
| Router Key PDU (Type 9) | ✅ (decode only) |
| Serial Notify | ✅ |
| Serial Query with incremental diffs | ✅ |
| Cache Reset on serial expiry | ✅ |
| Version negotiation and mismatch handling | ✅ |
| Error Report PDU (all defined error codes) | ✅ |
| VRP expiry (`expires` field) | ✅ |
| Multiple concurrent clients | ✅ |
| Dual stack (IPv4 + IPv6 transport) | ✅ |

### PDU Error Codes

All error codes defined in the protocol draft are implemented:

| Code | Name | Condition |
|---|---|---|
| 0 | Corrupt Data | Malformed PDU received |
| 1 | Internal Error | Server-side processing failure |
| 2 | No Data | Cache has no data to serve |
| 3 | Invalid Request | PDU type not valid in current state |
| 4 | Unsupported Version | Client version not in {1, 2} |
| 5 | Unsupported PDU Type | Received an unknown PDU type |
| 6 | Unknown Withdrawal | Withdrawal for a prefix not in cache |
| 7 | Duplicate Announcement | Same prefix announced twice |
| 8 | Unexpected Version | Client changed version mid-session |
| 9 | ASPA List Error | Malformed ASPA payload |
| 10 | Transport Error | Underlying connection fault |

---

## Features

**Incremental updates via serial history.** `rpkirtr2` maintains a ring buffer of the last 10 diff records. Clients that have missed up to 10 refresh cycles receive incremental add/withdraw updates rather than a full Cache Reset. Clients with a serial older than the history window receive a Cache Reset, triggering a full re-sync.

**ASPA support.** Full end-to-end ASPA (Autonomous System Provider Authorization) handling: fetches ASPA data from a configurable JSON feed, validates and deduplicates entries, computes incremental diffs, and sends ASPA PDUs to version 2 clients. Version 1 clients receive no ASPA PDUs — the server handles version-gating automatically.

**VRP expiry enforcement.** Each VRP in the upstream JSON feed carries an `expires` Unix timestamp. `rpkirtr2` filters out expired entries on every refresh cycle and on cold start, preventing stale data from reaching routers if the upstream validator pipeline stalls.

**Multiple upstream sources.** Separate, configurable URL lists for ROA and ASPA data. Fetches run concurrently. If one upstream fails, the server retains the previous dataset for that feed rather than issuing mass withdrawals for a transient error.

**Graceful shutdown.** `SIGTERM` and `SIGINT` trigger a clean shutdown with a configurable timeout (default 60 seconds). Active client sessions are allowed to drain. The server will not exit while clients are mid-stream unless the timeout fires.

**gRPC statistics API.** Exposes cache state — ROA count, ASPA count, current serial, last update time, connected client count, and per-upstream fetch health — via a gRPC interface. Suitable for integration with monitoring pipelines.

---

## Architecture

```
┌────────────────────────────────────────────────────┐
│                    rpkirtr2                        │
│                                                    │
│  ┌──────────────┐       ┌──────────────────────┐  │
│  │  Updater     │       │  Cache               │  │
│  │  (periodic)  │──────▶│  roas []ROA          │  │
│  │              │       │  aspas []ASPA        │  │
│  │  ROA URLs ──▶│       │  history [10]diff    │  │
│  │  ASPA URLs──▶│       │  serial uint32       │  │
│  └──────────────┘       └──────────┬───────────┘  │
│                                    │               │
│                          ┌─────────▼────────┐      │
│                          │  Client Handler  │      │
│                          │  (per TCP conn)  │      │
│                          │                  │      │
│                          │  Negotiate ver.  │      │
│                          │  Reset Query     │      │
│                          │  Serial Query    │      │
│                          │  Serial Notify   │      │
│                          └──────────────────┘      │
│                                                    │
│  ┌──────────────┐                                  │
│  │  gRPC API   │  GetStats()                       │
│  └──────────────┘                                  │
└────────────────────────────────────────────────────┘
         ▲                        ▼
  RPKI JSON feeds         BGP daemons / routers
  (rpki-client, etc.)     (BIRD, FRR, OpenBGPD, etc.)
```

### Refresh cycle

1. The updater goroutine wakes on a configurable interval (default 3600 seconds).
2. ROA and ASPA feeds are fetched concurrently via HTTP using a long-lived `http.Client`.
3. JSON is decoded via a streaming token-by-token decoder — the full response is never materialised in memory simultaneously with the processed dataset.
4. Entries are validated (RFC 6482 for ROAs, CustomerASN/ProviderASN rules for ASPAs), deduplicated in-place using sorted-slice comparison, and filtered for expiry.
5. A two-pointer sorted diff against the current cache produces the incremental add/withdraw lists.
6. The cache is updated under a single write lock; the diff is appended to the history ring buffer and the serial is incremented.
7. All connected clients receive a Serial Notify PDU.

### Diff history and serial handling

The server maintains a history ring buffer of 10 diff records, each keyed by `from` and `to` serial. When a client sends a Serial Query:

- If the client's serial matches the current serial: respond with an empty `CacheResponse` + `EndOfData` (already up to date).
- If the client's serial is in the history window: aggregate all diffs from that serial forward, cancel opposing add/withdraw pairs for the same prefix, and send the net result as an incremental update.
- If the client's serial is older than the history window, or the session ID does not match: send `CacheReset`. The client will follow up with a Reset Query to receive the full dataset.

---

## Getting Started

### Requirements

- Go 1.26 or later
- Network access to one or more RPKI JSON endpoints

### Build from source

```bash
git clone https://github.com/mellowdrifter/rpkirtr2.git
cd rpkirtr2
go build -o bin/rpkirtr2 ./cmd/rpkirtr2
```

### Quick run

```bash
./bin/rpkirtr2 \
  -rpki-url https://rpki.gin.ntt.net/api/export.json \
  -rpki-url https://console.rpki-client.org/vrps.json \
  -aspa-url https://console.rpki-client.org/rpki.json
```

The server binds to `:8282` (RTR) and `:50051` (gRPC) by default.

---

## Configuration

`rpkirtr2` accepts configuration from a YAML file, CLI flags, or both. CLI flags take precedence over file values.

### YAML configuration file

```yaml
listen_addr: ":8282"          # RTR listen address. Default: :8282
grpc_addr: ":50051"           # gRPC statistics listen address. Default: :50051
log_level: "info"             # Logging verbosity: debug | info | warn | error
refresh_interval: 3600        # Upstream fetch interval in seconds. Default: 3600

rpki_urls:                    # One or more ROA JSON feed URLs
  - "https://rpki.gin.ntt.net/api/export.json"
  - "https://console.rpki-client.org/vrps.json"

aspa_urls:                    # One or more ASPA JSON feed URLs (optional)
  - "https://console.rpki-client.org/rpki.json"
```

Run with a config file:

```bash
./bin/rpkirtr2 -config /etc/rpkirtr2/config.yaml
```

### CLI flags

| Flag | Default | Description |
|---|---|---|
| `-config` | — | Path to YAML configuration file |
| `-listen` | `:8282` | RTR TCP listen address |
| `-grpc-listen` | `:50051` | gRPC statistics listen address |
| `-loglevel` | `info` | Log level (`debug`, `info`, `warn`, `error`) |
| `-refresh` | `3600` | Upstream fetch interval in seconds |
| `-rpki-url` | *(see below)* | ROA JSON feed URL (repeatable) |
| `-aspa-url` | — | ASPA JSON feed URL (repeatable) |

If no `-rpki-url` is provided and no `rpki_urls` are set in the config file, the server falls back to:
- `https://rpki.gin.ntt.net/api/export.json`
- `https://console.rpki-client.org/vrps.json`

### Configuration precedence

```
CLI flags  >  YAML config file  >  built-in defaults
```

---

## Running in Production

### Systemd unit

```ini
[Unit]
Description=rpkirtr2 RPKI-to-Router Server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=rpkirtr2
ExecStart=/usr/local/bin/rpkirtr2 -config /etc/rpkirtr2/config.yaml
Restart=on-failure
RestartSec=5s

# Memory cap — set to ~90% of available RAM for this service.
# The GC targets staying below this; actual RSS will be lower at steady state.
Environment=GOMEMLIMIT=300MiB

# Harden the service
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadOnlyPaths=/etc/rpkirtr2

[Install]
WantedBy=multi-user.target
```

### Docker

```dockerfile
FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY . .
RUN go build -o /rpkirtr2 ./cmd/rpkirtr2

FROM alpine:3.20
COPY --from=builder /rpkirtr2 /usr/local/bin/rpkirtr2
COPY config.yaml /etc/rpkirtr2/config.yaml
EXPOSE 8282 50051
ENV GOMEMLIMIT=300MiB
ENTRYPOINT ["/usr/local/bin/rpkirtr2", "-config", "/etc/rpkirtr2/config.yaml"]
```

### Memory tuning

The server uses `GOGC=50` internally to collect garbage more aggressively than the Go default. Set `GOMEMLIMIT` in the environment to establish a soft RSS ceiling — the GC will work harder to stay under it.

| Deployment | Recommended `GOMEMLIMIT` |
|---|---|
| Single upstream, small feed | `200MiB` |
| Single upstream, full internet table | `350MiB` |
| Multiple upstreams | `500MiB` |
| Container with hard limit | 90% of container memory limit |

Peak memory occurs during the refresh cycle when the old and new datasets are briefly live simultaneously. Steady-state RSS is significantly lower.

### Signals

| Signal | Behaviour |
|---|---|
| `SIGTERM` | Graceful shutdown (60 second timeout) |
| `SIGINT` | Graceful shutdown (60 second timeout) |

---

## gRPC Statistics API

The server exposes a `GetStats` RPC on the configured gRPC address. The response includes:

| Field | Type | Description |
|---|---|---|
| `roa_count` | `uint32` | Number of valid ROAs currently in cache |
| `client_count` | `uint32` | Number of currently connected RTR clients |
| `serial` | `uint32` | Current cache serial number |
| `last_update` | `int64` | Unix timestamp of last successful cache refresh |
| `upstreams` | `[]UpstreamStatus` | Per-URL fetch health (see below) |

Each `UpstreamStatus` entry contains:

| Field | Type | Description |
|---|---|---|
| `url` | `string` | The upstream feed URL |
| `last_fetch_success` | `bool` | Whether the most recent fetch succeeded |
| `last_fetch_time` | `int64` | Unix timestamp of the most recent fetch attempt |
| `error_message` | `string` | Error detail if the last fetch failed |

Query with `grpcurl`:

```bash
grpcurl -plaintext localhost:50051 rpkirtr.v1.RPKIRTRService/GetStats
```

---

## Memory Management

`rpkirtr2` is designed for minimal memory overhead on both steady-state and refresh cycles.

**Streaming JSON decode.** Upstream feeds are decoded one entry at a time using Go's `json.Decoder` token API. The full JSON response body is never held in memory simultaneously with the processed dataset, eliminating the largest intermediate allocation present in naive implementations.

**Sorted-slice diff.** ROAs and ASPAs are stored in sorted order by a struct key. Incremental diffs are computed with a two-pointer linear scan — O(n) time, zero heap allocations. There are no per-refresh map allocations over the full dataset.

**In-place deduplication.** Deduplication is performed using `slice[:0]` reuse, recycling the backing array rather than allocating a new slice for the output.

**Struct-based cache keys.** The diff and lookup keys are compact structs rather than formatted strings, avoiding the cost of string allocation and GC pressure during large cache operations.

**Controlled GC.** `GOGC=50` is set at startup to trigger GC at a lower heap growth ratio than the default. Operators should also set `GOMEMLIMIT` to give the GC a target RSS ceiling. See [Memory tuning](#running-in-production).

---

## VRP Expiry

Each VRP in the upstream JSON feed (e.g. `rpki-client.org`) includes an `expires` Unix timestamp:

```json
{
  "prefix": "1.2.3.0/24",
  "maxLength": 24,
  "asn": 64496,
  "expires": 1750000000
}
```

The `expires` field is used as a pipeline health guard: if the upstream validator stops producing updated data, expiry timestamps will not advance and entries will age out of the cache, preventing routers from making routing decisions based on stale RPKI state.

`rpkirtr2` enforces expiry at two points:

**On cold start:** Expired entries are filtered before the first client ever connects. A server that was offline for an extended period will not serve stale data on restart.

**On each refresh cycle:** Entries whose `expires` timestamp has passed are excluded from the new dataset, appearing as withdrawals to connected clients in the next incremental diff.

Entries with `expires: 0` are treated as having no expiry constraint and are always served.

---

## Client Behaviour

### Version negotiation

The first byte read from each client connection is the RTR version byte. The server supports versions 1 and 2. Any other version results in an `UnsupportedVersion` Error Report PDU and connection close. Once a version is negotiated for a session, sending a PDU with a different version results in an `UnexpectedVersion` Error Report PDU and connection close.

### ASPA PDUs and protocol version

ASPA PDUs are only sent to clients that negotiated RTR version 2. Version 1 clients receive IPv4 Prefix, IPv6 Prefix, and End of Data PDUs only. The server handles this automatically — no client-side configuration is needed.

### Reset Query

A Reset Query triggers a full cache dump:

1. Server sends `CacheResponse` with the current session ID.
2. Server sends all current IPv4 Prefix, IPv6 Prefix, and (for v2 clients) ASPA PDUs with `Flags = Announce`.
3. Server sends `EndOfData` with the current serial and the RTR refresh/retry/expire intervals.

The `EndOfData` intervals conform to RFC 8210 bounds:

| Interval | Value | RFC 8210 Range |
|---|---|---|
| Refresh | 3600 s | 1 – 86400 s |
| Retry | 600 s | 1 – 7200 s |
| Expire | 7200 s | 600 – 172800 s |

### Serial Query

A Serial Query requests incremental updates since a given serial:

- **Session ID mismatch:** `CacheReset`. The client should follow up with a Reset Query.
- **Serial in history window:** `CacheResponse`, followed by net-aggregated diff PDUs, then `EndOfData`.
- **Serial not in history / too old:** `CacheReset`. The client should follow up with a Reset Query.
- **Serial matches current:** `CacheResponse` + immediate `EndOfData` (no changes).

### Read deadline

Connections have a read deadline applied on the initial PDU read. Stuck or slow clients that stop sending will be detected and cleaned up by the server.

### Mid-session errors

If a client sends a malformed or unexpected PDU after a session is established, the server responds with an `InvalidRequest` Error Report PDU and closes the connection.

---

## Testing

The test suite covers unit tests, integration tests, fuzz tests, and benchmarks.

### Run all tests

```bash
go test ./...
```

### Run with race detector

```bash
go test -race ./...
```

### Run integration tests only

```bash
go test ./clienttest/...
```

Integration tests spin up a real server on a random local port and exercise the full RTR protocol over TCP, including:

- Reset Query and Serial Query flows for both protocol versions
- Version negotiation and mismatch detection
- Malformed PDU handling (before and mid-session)
- VRP expiry filtering (cold start and incremental)
- Historical diff aggregation across multiple serials
- Serial history boundary and eviction (Cache Reset after >10 updates)
- ASPA end-to-end (announce and incremental diff)
- v1 client receives no ASPA PDUs
- `EndOfData` interval RFC compliance
- Graceful shutdown with active clients mid-stream
- Concurrent Reset Query stress (100 clients, 10 goroutines)

### Run fuzz tests

```bash
# Fuzz the PDU decoder
go test -fuzz=FuzzDecipherPDU ./internal/protocol/ -fuzztime=60s

# Fuzz the ROA JSON decoder
go test -fuzz=FuzzDecodeROAsJSON ./internal/server/ -fuzztime=60s

# Fuzz the ASPA JSON decoder
go test -fuzz=FuzzDecodeASPAsJSON ./internal/server/ -fuzztime=60s

# Fuzz the diff engine with invariant checking
go test -fuzz=FuzzMakeDiff ./internal/server/ -fuzztime=60s
go test -fuzz=FuzzMakeASPADiff ./internal/server/ -fuzztime=60s

# Fuzz GetSetOfValidatedROAs
go test -fuzz=FuzzGetSetOfValidatedROAs ./internal/server/ -fuzztime=60s
```

### Run benchmarks

```bash
go test -bench=BenchmarkMakeDiff ./internal/server/ -benchmem
```

---

## Upstream JSON Feed Format

`rpkirtr2` consumes RPKI JSON in the format produced by `rpki-client` and compatible validators.

**ROA feed:**

```json
{
  "roas": [
    {
      "prefix": "1.2.3.0/24",
      "maxLength": 24,
      "asn": "AS64496",
      "expires": 1750000000
    },
    {
      "prefix": "2001:db8::/32",
      "maxLength": 48,
      "asn": 64497,
      "expires": 0
    }
  ]
}
```

The `asn` field may be either a string (`"AS64496"`) or an integer (`64496`). Both forms are handled. The `expires` field is optional; entries without it (or with `expires: 0`) are never filtered for expiry.

**ASPA feed:**

```json
{
  "aspa": [
    {
      "customer": 64496,
      "providers": [
        {"asn": 64497},
        {"asn": 64498}
      ],
      "expires": 1750000000
    }
  ]
}
```

---

## License

MIT. See [LICENSE](LICENSE).