# Trbillo install bundle

Self-contained bundle to install Trbillo on a Linux amd64 server behind Caddy
at `https://your-domain.com/trbillo`.

## Contents

| File | Purpose |
|---|---|
| `trbillo` | Statically linked Linux amd64 binary |
| `static/` | Frontend assets (HTML/CSS/JS) |
| `trbillo.service` | systemd unit (installed to `/etc/systemd/system/`) |
| `install.sh` | Run as root to install/upgrade |
| `caddy-fragment.conf` | Snippet to drop into your `your-domain.com` Caddy site block |

## Install

```bash
# On your dev machine:
scp trbillo-install.tar.gz user@your-domain.com:~/

# On the server:
tar xzf trbillo-install.tar.gz
cd trbillo-install
sudo ./install.sh
```

That writes:
- `/opt/trbillo/trbillo` — binary
- `/opt/trbillo/static/` — assets
- `/var/lib/trbillo/` — SQLite database (owned by `trbillo` service user)
- `/etc/systemd/system/trbillo.service` — service unit

Then enables and starts the service. The Go server listens on `localhost:8080`
with `BASE_PATH=/trbillo`.

## Wire up Caddy

Add the two `handle` blocks from `caddy-fragment.conf` to your existing
`your-domain.com { ... }` block in `/etc/caddy/Caddyfile`, then:

```bash
sudo caddy validate --config /etc/caddy/Caddyfile
sudo systemctl reload caddy
```

Visit `https://your-domain.com/trbillo/` — the app should load.

## Upgrading

Drop in a new bundle and re-run `sudo ./install.sh`. The script stops the
service, replaces the binary and static files, and restarts. The SQLite
database in `/var/lib/trbillo/` is untouched.

## Useful commands

```bash
systemctl status trbillo
journalctl -u trbillo -f               # tail logs
sudo systemctl restart trbillo
sqlite3 /var/lib/trbillo/trbillo.db    # poke at the DB
```

## Configuration

The systemd unit sets these env vars. Edit `/etc/systemd/system/trbillo.service`
and `systemctl daemon-reload && systemctl restart trbillo` to change them.

| Env | Default in unit | Purpose |
|---|---|---|
| `BASE_PATH` | `/trbillo` | URL prefix the app serves under |
| `PORT` | `8080` | TCP port the Go server binds to |
| `DB_PATH` | `/var/lib/trbillo/trbillo.db` | SQLite file location |
| `STATIC_DIR` | `/opt/trbillo/static` | Frontend assets directory |
