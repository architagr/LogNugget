# loki-bench — zerolog vs LogNugget latency benchmark

Measures the **HTTP handler p99 latency impact** of zerolog (sync) vs LogNugget
(async) when logs are emitted to a real Loki instance over HTTP.

## Hypothesis

With a fast in-memory writer, zerolog wins (no ring overhead). Under real IO
latency (Loki HTTP push: 2–10 ms round-trip), LogNugget's async pipeline
decouples the caller from IO — handler latency tracks only CPU work, not log IO.

## Setup

### 1. Start Loki + Grafana

```bash
cd loki/
docker-compose up -d
# Loki ready at http://localhost:3100
# Grafana ready at http://localhost:3000
```

### 2. Build servers

```bash
go build -o bin/zerolog-server    ./cmd/zerolog-server
go build -o bin/lognugget-server  ./cmd/lognugget-server
```

### 3. Run servers

```bash
# Terminal 1
LOKI_URL=http://localhost:3100 ./bin/zerolog-server    # :8080

# Terminal 2
LOKI_URL=http://localhost:3100 ./bin/lognugget-server  # :8081
```

### 4. Load test (requires k6)

```bash
# zerolog
k6 run -e TARGET_URL=http://localhost:8080 load/k6.js

# LogNugget
k6 run -e TARGET_URL=http://localhost:8081 load/k6.js
```

## What each server does

- `GET /api/v1/work`: simulates ~1 ms of CPU work, logs 1 structured event per
  request (method, path, status, latency_us, trace_id), returns JSON.
- **zerolog-server**: synchronous — handler blocks on Loki HTTP POST before
  returning the response.
- **lognugget-server**: asynchronous — handler enqueues the log event to the
  lock-free ring buffer and returns immediately. LogNugget's consumer goroutine
  pushes to Loki independently.

## Expected results

| Server | p50 | p99 | Notes |
|--------|-----|-----|-------|
| zerolog | ~1 ms + IO | ~(1 ms + Loki RTT) | blocks on Loki write |
| LogNugget | ~1 ms | ~1.2 ms | async — Loki IO off the hot path |

Under local Loki (same machine, ~1 ms RTT), both should be close. Under a
remote Loki (5–10 ms RTT) the gap widens — LogNugget's p99 stays near 1 ms
while zerolog's p99 approaches 1 ms + IO RTT.

## Profiling

```bash
# Capture alloc profile at steady state
go tool pprof -alloc_space http://localhost:8080/debug/pprof/allocs
go tool pprof -alloc_space http://localhost:8081/debug/pprof/allocs
```
