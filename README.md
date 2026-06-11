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
- Invitation-code signups: an open `PUBLIC` code for 30-day trial
  accounts, plus admin-created teams with rotatable signup codes whose
  members can find each other when inviting people to boards
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

## Resetting a password

There is no self-service "forgot password" flow (the server sends no
email). Instead, the binary doubles as an admin CLI:

```bash
./trbillo -reset-password alice        # or go run . -reset-password alice
```

Accepts a username or email, prompts for the new password twice (input
hidden), and logs the user out of all existing sessions. It uses
`DB_PATH` to find the database, so on a production install run it as
the service user with the production path:

```bash
sudo -u trbillo DB_PATH=/var/lib/trbillo/trbillo.db \
  /opt/trbillo/trbillo -reset-password alice
```

Safe to run while the server is up. For scripted use, pipe the new
password as a single line on stdin.

## Teams and invitation codes

Creating an account requires an invitation code. Each code belongs to a
**team**: the code is the secret people type at signup, the team is who
they permanently become. Because they're separate, the admin can rotate
a team's code at any time (say, if it leaks or attracts spam) without
affecting existing members — only future signups need the new code.

- The **`PUBLIC` team** always exists (seeded at startup with code
  `PUBLIC`) and lets anybody register — that's what makes a deployment
  work as an open demo. PUBLIC accounts are **trial accounts**: an
  hourly server job deletes them 30 days after signup, *including any
  boards they own*, and they see a banner showing their deletion date.
  Getting spammed? Rotate the PUBLIC code. To keep a trial user, the
  admin can move them to a real team from the Users tab.
- **Other teams** are created by the admin (Teams tab) with a name
  (e.g. `Snakkos`) and a code (e.g. `LUNCHTIME`). Their accounts are
  permanent, and teammates see each other in the board invite dialog's
  "Your Team" list — by team name only, so people can be invited across
  boards without anyone learning a signup code.
- Accounts with **no team** (the admin user, accounts created by the
  admin without a team, and accounts that predate this feature) are
  permanent and belong to no team.

Deleting a team closes it for new signups; existing accounts keep their
team and visibility.

## Admin user

The username `admin` is reserved: nobody can register it, and the server
creates the account automatically at startup with an unusable password,
so it stays locked until you explicitly enable it:

```bash
./trbillo -set-admin     # creates admin if needed, prompts for a password
```

(`-reset-password admin` also works once the account exists.)

Logging in as `admin` opens an admin panel instead of the normal
dashboard, with two tabs:

- **Boards** — a sortable, filterable, searchable index of every board
  with owner and member counts. From here the admin can open any board
  in a strictly read-only view (no card/list editing, no drag-and-drop),
  inspect each board's membership, remove members, and transfer board
  ownership (the previous owner stays on the board as a member).
- **Users** — a sortable, searchable table of every account with its
  team. The admin can create users (optionally on a team), change a
  user's team (e.g. move someone from PUBLIC onto a real team to make
  them permanent), set passwords (logging that user out everywhere),
  and delete users. Deleting a user who still owns boards is blocked
  until their boards are reassigned or deleted.
- **Teams** — create teams (name + signup code), rotate a team's code,
  see member counts, and delete teams (PUBLIC is protected).

The admin account itself cannot be deleted, cannot own or join boards,
and cannot edit board content — it is a management account, not a
collaborator.

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
