# Trbillo Deployment Guide

How to build and install Trbillo on a Linux server behind Caddy.

Throughout this guide, `your-domain.com` is a placeholder — replace it
with the domain you own. The `/trbillo` URL prefix is also a deployment
choice: set `BASE_PATH=""` in the systemd unit and adjust the Caddy
config to mount at the root, or change `BASE_PATH=/whatever` to use a
different prefix.

## Architecture

- **Go server** binds `localhost:8080`, serves under URL prefix `/trbillo`
  (via `BASE_PATH` env var).
- **Caddy** terminates TLS for `your-domain.com` and reverse-proxies
  `/trbillo/*` to the Go server (preserving the prefix).
- **SQLite** database in `/var/lib/trbillo/trbillo.db`, owned by a dedicated
  `trbillo` system user.
- **systemd** keeps the service running and restarts on failure.

```
Browser → https://your-domain.com/trbillo/...
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

## Building and uploading the bundle

From a dev machine with Go installed, the one-shot path is:

```bash
./deploy.sh root@your-server     # or user@host
```

That cross-compiles the binary, bundles static assets + scaffolding,
tars it, and `scp`s the tarball to `~/` on the target.

`deploy.sh` reads the install scaffolding (systemd unit, Caddy snippet,
install script, README) from the tracked `deploy/` directory.

If you want the manual equivalent:

```bash
mkdir -p dist/trbillo-install
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
  go build -ldflags="-s -w" -o dist/trbillo-install/trbillo .
cp -r static dist/trbillo-install/static
cp deploy/install.sh deploy/trbillo.service \
   deploy/caddy-fragment.conf deploy/README.md dist/trbillo-install/
chmod +x dist/trbillo-install/install.sh
tar -czf dist/trbillo-install.tar.gz -C dist trbillo-install
```

The resulting `dist/trbillo-install.tar.gz` is ~4.4 MB and contains:

| File | Purpose |
|---|---|
| `trbillo` | Statically linked Linux amd64 binary (~10 MB, stripped) |
| `static/` | Frontend assets (HTML/CSS/JS) |
| `trbillo.service` | systemd unit (installs to `/etc/systemd/system/`) |
| `install.sh` | Idempotent installer, run as root |
| `caddy-fragment.conf` | Snippet to drop into `your-domain.com` Caddy site block |
| `README.md` | Quick reference |

## Installing on the server

### 1. Upload and run installer

If you used `./deploy.sh user@server` the tarball is already in `~/` on
the server. Otherwise upload manually:

```bash
scp dist/trbillo-install.tar.gz user@your-domain.com:~/
```

Then on the server:

```bash
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
`your-domain.com`, not `:80` or `http://...`).

Full `/etc/caddy/Caddyfile` for serving static files at the root and
proxying `/trbillo/*` to the Go server:

```caddy
your-domain.com {
    # Static file root for everything that isn't a /trbillo route
    root * /var/www/html

    # --- Trbillo (proxied to the Go server on localhost:8080) ---
    handle /trbillo {
        redir /trbillo/ 308
    }
    handle /trbillo/* {
        reverse_proxy localhost:8080
    }

    # --- Everything else: serve static files from /var/www/html ---
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
  sudo mkdir -p /var/www/html
  sudo chown -R caddy:caddy /var/www/html
  echo '<h1>your-domain.com</h1>' | sudo tee /var/www/html/index.html
  ```
- Adding another app later (e.g. `/blog`) is the same pattern: add another
  `handle /blog/*` block — order doesn't matter.

Validate and reload:

```bash
sudo caddy validate --config /etc/caddy/Caddyfile
sudo systemctl reload caddy
```

### 3. Verify

Visit `https://your-domain.com/trbillo/` — the Trbillo login page should load.
Create a test account, make a board, drop a card on it. Open the same
board in two browsers to confirm real-time WebSocket sync.

## Upgrading

Drop in a newer bundle on the server and re-run `sudo ./install.sh`. The
script stops the service, replaces the binary and static files, and
restarts. The SQLite DB in `/var/lib/trbillo/` is untouched.

## Resetting a user's password

The binary doubles as an admin CLI:

```bash
sudo -u trbillo DB_PATH=/var/lib/trbillo/trbillo.db \
  /opt/trbillo/trbillo -reset-password alice
```

Accepts a username or email. Prompts for the new password twice (input
hidden), writes a fresh bcrypt hash, and logs the user out of all
existing sessions. Safe to run while the service is up — it opens the
same SQLite file alongside the server and waits out any write lock.

For scripted use, pipe the password on stdin (one line):

```bash
echo 'new-password' | sudo -u trbillo DB_PATH=/var/lib/trbillo/trbillo.db \
  /opt/trbillo/trbillo -reset-password alice
```

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
