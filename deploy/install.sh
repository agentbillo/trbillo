#!/usr/bin/env bash
# Trbillo installer
#
# Usage: sudo ./install.sh
#
# Installs:
#   /opt/trbillo/trbillo           (binary, root-owned, read-only to service)
#   /opt/trbillo/static/           (static assets, root-owned)
#   /var/lib/trbillo/              (SQLite DB, owned by trbillo user)
#   /etc/systemd/system/trbillo.service
#
# Idempotent: safe to re-run for upgrades. Service is stopped before files
# are replaced and restarted after.

set -euo pipefail

BUNDLE_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &> /dev/null && pwd)"
INSTALL_DIR=/opt/trbillo
DATA_DIR=/var/lib/trbillo
SERVICE_FILE=/etc/systemd/system/trbillo.service
SERVICE_USER=trbillo

if [[ $EUID -ne 0 ]]; then
  echo "This script must be run as root (try: sudo $0)" >&2
  exit 1
fi

for f in trbillo trbillo.service static/index.html; do
  if [[ ! -e "$BUNDLE_DIR/$f" ]]; then
    echo "Missing bundle file: $f" >&2
    exit 1
  fi
done

echo "==> Creating service user '$SERVICE_USER' (if missing)"
if ! id -u "$SERVICE_USER" >/dev/null 2>&1; then
  useradd --system --no-create-home --shell /usr/sbin/nologin "$SERVICE_USER"
fi

echo "==> Stopping trbillo (if running)"
systemctl stop trbillo 2>/dev/null || true

echo "==> Creating directories"
install -d -o root  -g root  -m 0755 "$INSTALL_DIR"
install -d -o "$SERVICE_USER" -g "$SERVICE_USER" -m 0750 "$DATA_DIR"

echo "==> Installing binary -> $INSTALL_DIR/trbillo"
install -o root -g root -m 0755 "$BUNDLE_DIR/trbillo" "$INSTALL_DIR/trbillo"

echo "==> Installing static assets -> $INSTALL_DIR/static"
rm -rf "$INSTALL_DIR/static"
cp -r "$BUNDLE_DIR/static" "$INSTALL_DIR/static"
chown -R root:root "$INSTALL_DIR/static"
find "$INSTALL_DIR/static" -type d -exec chmod 0755 {} \;
find "$INSTALL_DIR/static" -type f -exec chmod 0644 {} \;

echo "==> Installing systemd unit -> $SERVICE_FILE"
install -o root -g root -m 0644 "$BUNDLE_DIR/trbillo.service" "$SERVICE_FILE"

echo "==> Reloading systemd, enabling and starting trbillo"
systemctl daemon-reload
systemctl enable trbillo
systemctl restart trbillo

sleep 1
if systemctl is-active --quiet trbillo; then
  echo
  echo "trbillo is running."
  echo "  status:  systemctl status trbillo"
  echo "  logs:    journalctl -u trbillo -f"
  echo "  listens: localhost:8080 (BASE_PATH=/trbillo)"
  echo
  echo "Next: add the contents of caddy-fragment.conf into your site block"
  echo "in /etc/caddy/Caddyfile, then:"
  echo "  sudo caddy validate --config /etc/caddy/Caddyfile"
  echo "  sudo systemctl reload caddy"
else
  echo
  echo "trbillo failed to start. Recent logs:" >&2
  journalctl -u trbillo --no-pager -n 30 >&2 || true
  exit 1
fi
