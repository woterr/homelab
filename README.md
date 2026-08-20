# homelab

Homelab infrastructure observability system.

homelab (π) is a lightweight, single-node observability daemon and dashboard for a
home server / Raspberry Pi. It collects real system, network, storage and service
metrics, persists them to SQLite with downsampled history, and presents a custom
instrumentation-style web interface.

```
DISCOVER → COLLECT → NORMALIZE → STORE → VISUALIZE
```

## Components

| Path       | What it is                                             |
|------------|--------------------------------------------------------|
| `backend/` | Go daemon: collectors, SQLite MetricStore, events, rules, ntfy notifications, REST + WebSocket API |
| `frontend/`| React + TypeScript + Vite dashboard (custom design system, dark/light themes, bespoke D3 charts, network topology) |
| `data/`    | Runtime SQLite database (gitignored)                   |

## Development

Requirements: Go 1.24+, Node 20+, npm.

```sh
# Backend in demo mode (deterministic simulated host) + vite dev server
make dev-demo

# Backend in real mode, collecting from the current Linux host
make dev
```

The frontend dev server proxies `/api` (including the WebSocket at `/api/v1/ws`)
to the backend on `:8000`.

Useful commands:

```sh
make backend       # run backend only (real mode)
make test          # go test + vitest
make lint          # gofmt + go vet + eslint
make build         # cross-compile arm64 + amd64 binaries
```

## Deployment (Docker Compose)

```sh
# Build the frontend and an ARM64 backend binary
cd frontend && npm run build
cd ../backend && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o bin/homelab-arm64 ./cmd/homelab

# Ship to the target host and bring it up on any free port (e.g. 8090)
rsync -a ../frontend/dist ../Caddyfile user@host:homelab/
rsync -a bin/homelab-arm64 user@host:homelab/bin/homelab
ssh user@host 'cd homelab && docker compose up -d'

# UI:      http://<host>:8090
# API:     http://<host>:8090/api/v1
```

The backend mounts `/proc`, `/sys` and the Docker socket read-only so collectors
can observe the host. The Caddy container serves the built frontend and reverse
proxies `/api` (REST + the `/api/v1/ws` WebSocket). The Caddyfile binds to `:80`
inside its container; map it to any free host port you like.

## Backend CLI

```
homelab [--demo] [--data-dir ./data] [--listen :8000]
         [--collect-interval 5s] [--retention 720h]
         [--ntfy-topic homelab] [--ntfy-server https://ntfy.sh]
```

## API surface (v1)

```
GET  /api/v1/system                    GET  /api/v1/system/cpu
GET  /api/v1/system/memory             GET  /api/v1/system/temperature
GET  /api/v1/network/interfaces        GET  /api/v1/network/devices
GET  /api/v1/network/topology          GET  /api/v1/storage
GET  /api/v1/storage/disks             GET  /api/v1/services
GET  /api/v1/services/:id/logs         GET  /api/v1/events
GET  /api/v1/metrics/series            GET  /api/v1/metrics/query
GET  /api/v1/rules                     POST /api/v1/rules
GET  /api/v1/settings                  PUT  /api/v1/settings
POST /api/v1/notify/test
WS   /api/v1/ws
```

## Architecture notes

- **MetricStore** is an interface (`Append`, `Query`, `ListSeries`, `Trim`); V1 is
  SQLite with batched writes, bucketed query aggregation and rollup compaction.
  A Prometheus-backed implementation can be swapped in later without touching the
  API or frontend.
- **Collectors** implement a small `Collector` interface and are registered in a
  scheduler. V1: system, cpu, memory, temperature, filesystem, disk, network,
  devices, services (Docker + systemd), power. A deterministic demo driver serves
  the same models in `--demo` mode.
- **Events** flow through an in-process bus, are persisted, broadcast over WS, and
  feed the threshold **rules** engine, which can fire **ntfy** notifications.
- The frontend never touches Linux files or sockets; all access is via the Go API.