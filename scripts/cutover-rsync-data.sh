#!/usr/bin/env bash
# Rsync School Nanny data to another host (Ryzen cutover helper).
# Usage:
#   sudo ./scripts/cutover-rsync-data.sh user@ryzen-host
# Optional:
#   DATA_SRC=/srv/school-nanny/data DEST_PATH=/srv/school-nanny/data ./scripts/cutover-rsync-data.sh user@host
set -euo pipefail

if [[ "${EUID}" -ne 0 ]]; then
  echo "run as root (needs to read ${DATA_SRC:-/srv/school-nanny/data})" >&2
  exit 1
fi

if [[ $# -lt 1 ]]; then
  echo "usage: $0 user@remote-host" >&2
  exit 1
fi

REMOTE="$1"
SRC="${DATA_SRC:-/srv/school-nanny/data}"
DEST="${DEST_PATH:-/srv/school-nanny/data}"

if [[ ! -d "$SRC" ]]; then
  echo "missing source data dir: $SRC" >&2
  exit 1
fi

echo "rsync $SRC/ -> ${REMOTE}:${DEST}/"
ssh "$REMOTE" "sudo mkdir -p '$DEST' && sudo chown -R 65532:65532 '$(dirname "$DEST")'"
rsync -aHAX --delete --info=progress2 \
  -e ssh \
  "$SRC/" \
  "$REMOTE:$DEST/"
ssh "$REMOTE" "sudo chown -R 65532:65532 '$DEST'"
echo "done. Point Coolify on the remote at $DEST → /data and repoint LAN DNS."
