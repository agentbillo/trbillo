#!/bin/sh
# Build, upload to the trbillo.com server and restart.
#
# That server keeps everything under /home/sites/trbillo (see the unit
# installed there), not the /opt + /var/lib layout that deploy.sh and
# install.sh set up. Don't run install.sh on it: it would replace the unit
# with one pointing at a different port, path and database.
#
# The login user (ubuntu) can't write under /home/sites, so the remote end of
# rsync runs as the service user, which also leaves the files owned by it.
set -e
cd "$(dirname "$0")/.."
HOST=ubuntu@ovh.billo.com
DIR=/home/sites/trbillo
AS_SVC="sudo -u billosrv rsync"

mkdir -p dist
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o dist/trbillo .
chmod 755 dist/trbillo

rsync -av --rsync-path="$AS_SVC" dist/trbillo ${HOST}:${DIR}/bin/
rsync -rav --delete --rsync-path="$AS_SVC" static/ ${HOST}:${DIR}/static/

ssh ${HOST} sudo systemctl restart trbillo
