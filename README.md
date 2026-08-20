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
| `backend/` | Go daemon: collectors, SQLite MetricStore, events, rules, notifications (ntfy/Discord/SMTP/SMS), REST + WebSocket API |
| `frontend/`| React + TypeScript + Vite dashboard (custom design system, dark/light themes, bespoke D3 charts, network topology) |
| `deploy/`  | Systemd units + Caddyfile + install script for the Raspberry Pi (native, no Docker) |
| `Caddyfile`| Standalone Caddy config for the Pi web layer (serves `frontend/dist`, proxies `/api`) |
| `docker-compose.yml` | Optional container deployment (backend + Caddy) for other hosts |
| `data/`    | Runtime SQLite database (gitignored)                   |

`frontend/dist/` and `backend/bin/homelab-arm64` are committed so the Pi can run
from a plain `git pull` with no Node/Go toolchain.

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
make backend          # run backend only (real mode)
make test             # go test + vitest
make lint             # gofmt + go vet + eslint
make build            # cross-compile arm64 + amd64 binaries
make build-linux-arm64 # build backend/bin/homelab-arm64 for the Pi
```

## Deployment (Raspberry Pi, native systemd)

The Pi runs homelab as two systemd services with no Docker, all from `~/homelab`
(`/home/user/homelab`):

- **`homelab-backend`** — the Go daemon running as the `user` account
  (`docker` + `video` groups, `CAP_NET_RAW`), listening on `127.0.0.1:8000`.
  Metrics live in `~/homelab/data` (`homelab.db`).
- **`homelab-caddy`** — a *separate* Caddy instance listening on **:8090**
  (the NAS's own Caddy on :80 is untouched). Serves `~/homelab/frontend/dist`
  and proxies `/api` + the `/api/v1/ws` WebSocket to the backend.

Deploy a new release:

```sh
# 1. Local machine: build artifacts and publish
make build-linux-arm64
cd frontend && npm run build
git add -A && git commit -m "release: ..." && git push

# 2. On the Pi (idempotent; pulls the repo and reloads services)
sudo bash ~/homelab/deploy/systemd/install-pi.sh
```

The install script:

1. clones/pulls the repo to `~/homelab` (uses the Pi's GitHub key; a non-git
   `~/homelab` from an old Docker layout is moved aside to `~/homelab.old`)
2. ensures the `user` account is in the `docker` and `video` groups
3. prepares `~/homelab/data` (migrates a leftover `/var/lib/homelab` if present)
4. installs the two systemd units and enables them
5. seeds `/etc/homelab.env` on first run (notification defaults)

Configuration lives in `/etc/homelab.env` (seeded from
`deploy/systemd/homelab.env.example`): ntfy server/topic, Discord webhook, SMTP
host/credentials, SMS gateway webhook. Channel settings saved from the Settings
page are stored in the database and layered on top of these defaults.

**Firewall:** the Pi uses UFW with a default-deny INPUT policy. Allow the
dashboard from your LAN:

```sh
sudo ufw allow from 192.168.29.0/24 to any port 8090 proto tcp
```

Then open `http://<pi-ip>:8090`.

Useful Pi commands:

```sh
journalctl -fu homelab-backend   # backend logs
journalctl -fu homelab-caddy     # web layer logs
sudo systemctl restart homelab-backend
```

### Container deployment (other hosts)

For a non-Pi host you can still run the whole stack in Docker:

```sh
docker compose up -d --build
```

The compose file runs the backend with `/proc`, `/sys` and the Docker socket
bind-mounted read-only so collectors observe the host, plus a Caddy container
serving the frontend and proxying `/api`.

## Backend CLI

```
homelab [--demo] [--data-dir ./data] [--listen :8000]
         [--host-proc /proc] [--host-sys /sys] [--host-root ""]
         [--host-docker /var/run/docker.sock]
         [--collect-interval 5s] [--retention 720h]
         [--rollup-window 1h] [--rollup-interval 5m]
         [--device-sweep] [--device-sweep-every 5m]
         [--ntfy-server https://ntfy.sh] [--ntfy-topic homelab]
         [--discord-webhook URL] [--smtp-host HOST] [--smtp-port 587]
         [--smtp-user U] [--smtp-pass P] [--smtp-from ADDR] [--smtp-to ADDRS]
         [--phone-webhook URL] [--phone-to NUMBER]
```

Every flag has an `HOST_*`/`NTFY_*`/`DISCORD_*`/`SMTP_*`/`PHONE_*` environment
equivalent (see `deploy/systemd/homelab.env.example`).

## API surface (v1)

```
GET  /api/v1/system                  GET  /api/v1/system/cpu
GET  /api/v1/system/memory           GET  /api/v1/system/temperature
GET  /api/v1/network/interfaces      GET  /api/v1/network/devices
GET  /api/v1/network/topology        GET  /api/v1/storage
GET  /api/v1/storage/disks           GET  /api/v1/power
GET  /api/v1/services                GET  /api/v1/services/:id/logs
GET  /api/v1/events                  GET  /api/v1/logs
GET  /api/v1/metrics/series          GET  /api/v1/metrics/query
GET  /api/v1/metrics/latest          GET  /api/v1/rules
POST /api/v1/rules                   DELETE /api/v1/rules/:id
GET  /api/v1/settings                PUT  /api/v1/settings/:key
GET  /api/v1/notify/providers        POST /api/v1/notify/test
GET  /api/v1/notify/config           PUT  /api/v1/notify/config
GET  /api/v1/internet                POST /api/v1/internet/speed
WS   /api/v1/ws
```

`POST /api/v1/internet/speed` runs a real Cloudflare download/upload test
(latency, download, upload, jitter); `GET /api/v1/internet` returns the cached
result so the dashboard doesn't re-test on every load.

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
  feed the threshold **rules** engine, which can fire notifications (ntfy, Discord,
  email, SMS) configured per-channel from the Settings page.
- The frontend never touches Linux files or sockets; all access is via the Go API.