# Trbillo Deployment Guide

How to build and install Trbillo on a Linux server behind Caddy at
`https://slowbase.com/trbillo`.

## Architecture

- **Go server** binds `localhost:8080`, serves under URL prefix `/trbillo`
  (via `BASE_PATH` env var).
- **Caddy** terminates TLS for `slowbase.com` and reverse-proxies
  `/trbillo/*` to the Go server (preserving the prefix).
- **SQLite** database in `/var/lib/trbillo/trbillo.db`, owned by a dedicated
  `trbillo` system user.
- **systemd** keeps the service running and restarts on failure.

```
Browser → https://slowbase.com/trbillo/...
            │
            ▼
          Caddy (:443, TLS)
            │  reverse_proxy /trbillo/* → localhost:8080
            ▼
          trbillo.service (User=trbillo, BASE_PATH=/trbillo)
            │
            ▼
          /var/lib/trbillo/trbillo.db
```

## Building the install bundle

From a dev machine with Go installed:

```bash
# Cross-compile static Linux amd64 binary
mkdir -p dist/trbillo-install
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
  go build -ldflags="-s -w" -o dist/trbillo-install/trbillo .

# Copy static assets and bundle scaffolding (already checked into dist
# scaffolding files: trbillo.service, caddy-fragment.conf, install.sh,
# README.md — copy from a prior bundle or recreate)
cp -r static dist/trbillo-install/static

# Tar it up
cd dist && tar czf trbillo-install.tar.gz trbillo-install
```

The resulting `dist/trbillo-install.tar.gz` is ~4.4 MB and contains:

| File | Purpose |
|---|---|
| `trbillo` | Statically linked Linux amd64 binary (~10 MB, stripped) |
| `static/` | Frontend assets (HTML/CSS/JS) |
| `trbillo.service` | systemd unit (installs to `/etc/systemd/system/`) |
| `install.sh` | Idempotent installer, run as root |
| `caddy-fragment.conf` | Snippet to drop into `slowbase.com` Caddy site block |
| `README.md` | Quick reference |

## Installing on the server

### 1. Upload and run installer

```bash
# From dev machine
scp dist/trbillo-install.tar.gz user@slowbase.com:~/

# On server
tar xzf trbillo-install.tar.gz
cd trbillo-install
sudo ./install.sh
```

`install.sh` does the following (idempotent — safe to re-run for upgrades):

1. Creates `trbillo` system user (no shell, no home).
2. Stops the service if running.
3. Creates dirs:
   - `/opt/trbillo` (root-owned, 0755) — binary + static
   - `/var/lib/trbillo` (trbillo-owned, 0750) — SQLite DB
4. Installs binary → `/opt/trbillo/trbillo`
5. Installs static assets → `/opt/trbillo/static/`
6. Installs systemd unit → `/etc/systemd/system/trbillo.service`
7. `systemctl daemon-reload && enable && restart trbillo`
8. Verifies service came up; dumps recent logs on failure.

After this, the Go server is listening on `localhost:8080` with
`BASE_PATH=/trbillo`. It is **not** yet reachable from the internet — that
needs Caddy.

### 2. Wire up Caddy

Caddy provisions a Let's Encrypt cert and handles HTTP→HTTPS redirect
automatically as long as the site is named with a real domain (e.g.
`slowbase.com`, not `:80` or `http://...`).

Full `/etc/caddy/Caddyfile` for serving static files at the root and
proxying `/trbillo/*` to the Go server:

```caddy
slowbase.com {
    # Static file root for everything that isn't a /trbillo route
    root * /var/www/slowbase

    # --- Trbillo (proxied to the Go server on localhost:8080) ---
    handle /trbillo {
        redir /trbillo/ 308
    }
    handle /trbillo/* {
        reverse_proxy localhost:8080
    }

    # --- Everything else: serve static files from /var/www/slowbase ---
    handle {
        file_server
        # For SPA-style fallback, uncomment:
        # try_files {path} {path}.html /index.html
    }

    encode zstd gzip
}
```

Notes:

- `handle` blocks are mutually exclusive and Caddy matches by specificity,
  not order, so `/trbillo/*` always wins over the catch-all `handle {}`.
- `reverse_proxy` upgrades WebSockets automatically, so `/trbillo/api/ws`
  and `/trbillo/api/ws/user` work without extra config.
- Make sure the static root exists and is readable by the `caddy` user:
  ```bash
  sudo mkdir -p /var/www/slowbase
  sudo chown -R caddy:caddy /var/www/slowbase
  echo '<h1>slowbase.com</h1>' | sudo tee /var/www/slowbase/index.html
  ```
- Adding another app later (e.g. `/blog`) is the same pattern: add another
  `handle /blog/*` block — order doesn't matter.

Validate and reload:

```bash
sudo caddy validate --config /etc/caddy/Caddyfile
sudo systemctl reload caddy
```

### 3. Verify

Visit `https://slowbase.com/trbillo/` — the Trbillo login page should load.
Create a test account, make a board, drop a card on it. Open the same
board in two browsers to confirm real-time WebSocket sync.

## Upgrading

Drop in a newer bundle on the server and re-run `sudo ./install.sh`. The
script stops the service, replaces the binary and static files, and
restarts. The SQLite DB in `/var/lib/trbillo/` is untouched.

## Useful commands

```bash
systemctl status trbillo
journalctl -u trbillo -f               # tail logs
sudo systemctl restart trbillo
sudo -u trbillo sqlite3 /var/lib/trbillo/trbillo.db
```

## Configuration

Edit `/etc/systemd/system/trbillo.service` and then
`sudo systemctl daemon-reload && sudo systemctl restart trbillo`.

| Env | Default | Purpose |
|---|---|---|
| `BASE_PATH` | `/trbillo` | URL prefix the app serves under (empty = root) |
| `PORT` | `8080` | TCP port the Go server binds to (localhost only) |
| `DB_PATH` | `/var/lib/trbillo/trbillo.db` | SQLite file location |
| `STATIC_DIR` | `/opt/trbillo/static` | Frontend assets directory |

## Backups

The only persistent state is the SQLite DB:

```bash
sudo -u trbillo sqlite3 /var/lib/trbillo/trbillo.db ".backup '/var/backups/trbillo-$(date +%F).db'"
```

A nightly cron with the above command (rotating, e.g., last 14 days) is
sufficient for most deployments.
