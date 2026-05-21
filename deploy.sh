#!/usr/bin/env bash
# Build the Trbillo Linux installer bundle and upload it to a server via scp.
#
# Usage: ./deploy.sh <scp-target>
# Example: ./deploy.sh root@1.2.3.4
#
# Produces dist/trbillo-install.tar.gz and copies it to <scp-target>:~/.
# Once uploaded, finish the install on the server with:
#   ssh <scp-target>
#   tar xzf trbillo-install.tar.gz && cd trbillo-install && sudo ./install.sh
#
# See DEPLOY.md for the full deployment guide.

set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "Usage: $0 <scp-target>" >&2
  echo "Example: $0 root@1.2.3.4" >&2
  exit 1
fi

TARGET="$1"

REPO_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &> /dev/null && pwd)"
cd "$REPO_DIR"

DIST_DIR="dist/trbillo-install"
TARBALL="dist/trbillo-install.tar.gz"

echo "==> Cleaning previous bundle"
rm -rf "$DIST_DIR" "$TARBALL"
mkdir -p "$DIST_DIR"

echo "==> Cross-compiling binary for linux/amd64"
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
  go build -ldflags="-s -w" -o "$DIST_DIR/trbillo" .

echo "==> Bundling static assets"
cp -r static "$DIST_DIR/static"

echo "==> Bundling install scaffolding"
cp deploy/install.sh         "$DIST_DIR/install.sh"
cp deploy/trbillo.service    "$DIST_DIR/trbillo.service"
cp deploy/caddy-fragment.conf "$DIST_DIR/caddy-fragment.conf"
cp deploy/README.md          "$DIST_DIR/README.md"
chmod +x "$DIST_DIR/install.sh"

echo "==> Creating tarball"
tar -czf "$TARBALL" -C dist trbillo-install
SIZE=$(du -h "$TARBALL" | cut -f1)
echo "    $TARBALL ($SIZE)"

echo "==> Uploading to $TARGET"
scp "$TARBALL" "$TARGET"

echo
echo "Done. To install or upgrade on the server:"
echo "  ssh $TARGET"
echo "  tar xzf trbillo-install.tar.gz && cd trbillo-install && sudo ./install.sh"
