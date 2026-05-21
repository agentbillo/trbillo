# Trbillo

A small, self-hosted, real-time collaborative Kanban board. Single Go
binary, SQLite for storage, no JavaScript build step, no external services.

## Features

- Boards, lists, cards with drag-and-drop reordering and moves between
  lists
- Cards have a title, body, link, due date, labels, assignees, checklist,
  and threaded comments
- Real-time multi-user sync over WebSockets
- Per-user accounts with bcrypt-hashed passwords, server-side sessions,
  CSRF protection, and Origin-based same-site enforcement
- Themable UI (dark, light, autumn, spring)
- Mountable under a URL subpath (e.g. `/trbillo`) via `BASE_PATH`, so it
  can sit alongside other apps on one domain
- Statically linked single binary (~10 MB) — drop it onto any Linux box

## Quick start (local dev)

```bash
go run .
```

Then open <http://localhost:8080>. The SQLite file is created at
`./trbillo.db` on first run. Register an account on the auth screen and
you're in.

Environment variables (all optional):

| Env | Default | Purpose |
|---|---|---|
| `BASE_PATH` | empty | URL prefix to mount under (`/trbillo`, etc.) |
| `PORT` | `8080` | TCP port to bind on `localhost` |
| `DB_PATH` | `./trbillo.db` | SQLite file location |
| `STATIC_DIR` | `./static` | Frontend assets directory |

## Production deployment

See [DEPLOY.md](DEPLOY.md) for the full guide: cross-compile, systemd
unit, Caddy reverse-proxy config (automatic HTTPS), backups, upgrades.

The short version:

```bash
./deploy.sh user@your-server
# then on the server:
ssh user@your-server
tar xzf trbillo-install.tar.gz && cd trbillo-install && sudo ./install.sh
```

## Project layout

```
main.go           HTTP routing, env config, static-asset templating
handlers.go       REST + WebSocket handlers, middleware, validation
db.go             SQLite schema, migrations, query helpers
models.go         Domain types
ws.go             WebSocket hub: per-board and per-user broadcasts
static/           HTML / CSS / JS (no build step)
deploy/           systemd unit, Caddy snippet, install.sh, README
deploy.sh         Build + scp the install bundle
DEPLOY.md         Production deployment guide
```

## Tech

- **Backend**: Go 1.22+ standard library router, `gorilla/websocket`,
  `modernc.org/sqlite` (CGO-free, statically linkable)
- **Frontend**: plain JavaScript (no build step), native CSS, native
  `<dialog>` modals
- **Database**: SQLite — single file, no external DB needed

## License

MIT — see [LICENSE](LICENSE).
