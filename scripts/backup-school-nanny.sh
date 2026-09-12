#!/usr/bin/env bash
# Nightly backup of the family's records plus a small snapshot of how this host
# is wired together.
#
# Two archives per run, both under ARCHIVE_ROOT:
#   data/school-nanny-data-YYYYMMDD-HHMMSS.tar.gz   the live DATA_DIR
#   infra/school-nanny-infra-YYYYMMDD-HHMMSS.tar.gz config files and notes
#
# The app container is stopped for the few seconds the data tar runs: SQLite in
# WAL mode does not promise a coherent snapshot of a database being written to,
# and a torn backup is worse than a short outage at 3am. The container is
# started again on every exit path, including failures.
#
# TLS private material is deliberately left out of the infra archive. The mkcert
# CA and leaf are a temporary proof that HTTPS works on school-nanny.home; the
# real deployment will use a public CA, so there is nothing here worth the risk
# of copying private keys into a nightly tarball.
#
# Run: sudo ./scripts/backup-school-nanny.sh
set -euo pipefail

DATA_DIR="${DATA_DIR:-/srv/school-nanny/data}"
ARCHIVE_ROOT="${ARCHIVE_ROOT:-/srv/school-nanny/archives}"
KEEP_DAYS="${KEEP_DAYS:-7}"
APP_LABEL="${APP_LABEL:-coolify.resourceName=school-nanny-local}"

DATA_ARCHIVE_DIR="${ARCHIVE_ROOT}/data"
INFRA_ARCHIVE_DIR="${ARCHIVE_ROOT}/infra"
STAMP=$(date +%Y%m%d-%H%M%S)

if [[ "$(id -u)" -ne 0 ]]; then
  echo "Run as root: sudo $0" >&2
  exit 1
fi

if [[ ! -d "${DATA_DIR}" ]]; then
  echo "No data folder at ${DATA_DIR}" >&2
  exit 1
fi

# The archives live beside the data folder, never inside it: a backup that
# contains every previous backup grows without bound.
case "${ARCHIVE_ROOT}/" in
  "${DATA_DIR}"/*)
    echo "ARCHIVE_ROOT must not be inside DATA_DIR" >&2
    exit 1
    ;;
esac

install -d -m 700 "${ARCHIVE_ROOT}" "${DATA_ARCHIVE_DIR}" "${INFRA_ARCHIVE_DIR}"

STOPPED_CONTAINERS=""

start_app() {
  if [[ -n "${STOPPED_CONTAINERS}" ]]; then
    echo "==> Starting the app again..."
    # shellcheck disable=SC2086
    docker start ${STOPPED_CONTAINERS} >/dev/null || true
    STOPPED_CONTAINERS=""
  fi
}
trap start_app EXIT

echo "==> Stopping the app so the database is not written to mid-copy..."
if docker info >/dev/null 2>&1; then
  STOPPED_CONTAINERS=$(docker ps -q --filter "label=${APP_LABEL}" | tr '\n' ' ')
  if [[ -n "${STOPPED_CONTAINERS}" ]]; then
    # shellcheck disable=SC2086
    docker stop ${STOPPED_CONTAINERS} >/dev/null
  else
    echo "    (nothing running for ${APP_LABEL} — backing up the files as they are)"
  fi
else
  echo "    (docker is not answering — backing up the files as they are)" >&2
fi

DATA_ARCHIVE="${DATA_ARCHIVE_DIR}/school-nanny-data-${STAMP}.tar.gz"
echo "==> Archiving ${DATA_DIR}..."
tar -czf "${DATA_ARCHIVE}.partial" -C "$(dirname "${DATA_DIR}")" "$(basename "${DATA_DIR}")"
mv "${DATA_ARCHIVE}.partial" "${DATA_ARCHIVE}"
chmod 600 "${DATA_ARCHIVE}"

start_app

# --- Infra snapshot -------------------------------------------------------
# Small enough to take every night, and it is the part that is tedious rather
# than impossible to reconstruct by hand.
STAGE=$(mktemp -d)
cleanup_stage() { rm -rf "${STAGE}"; }
trap 'start_app; cleanup_stage' EXIT

INFRA_STAGE="${STAGE}/school-nanny-infra-${STAMP}"
mkdir -p "${INFRA_STAGE}/files"

# One unreadable config is not a reason to lose the night's backup, so a failed
# copy is reported and skipped rather than fatal.
copy_if_present() {
  local src="$1"
  if [[ -r "${src}" ]]; then
    local dest="${INFRA_STAGE}/files${src}"
    mkdir -p "$(dirname "${dest}")"
    cp "${src}" "${dest}" || echo "    (could not copy ${src})" >&2
  fi
}

copy_if_present /etc/dnsmasq.d/school-nanny.conf
copy_if_present /etc/systemd/system/dnsmasq.service.d/retry-on-boot.conf
copy_if_present /etc/NetworkManager/dispatcher.d/99-school-nanny-dnsmasq
copy_if_present /etc/systemd/system/school-nanny-backup.service
copy_if_present /etc/systemd/system/school-nanny-backup.timer
copy_if_present /data/coolify/source/.env

{
  echo "# School Nanny host notes — ${STAMP}"
  echo
  echo "## Firewall"
  ufw status numbered 2>/dev/null || echo "(ufw not available)"
  echo
  echo "## Coolify-managed containers"
  docker ps -a --filter label=coolify.managed=true \
    --format 'table {{.Names}}\t{{.Status}}\t{{.Label "coolify.resourceName"}}' 2>/dev/null \
    || echo "(docker not available)"
  echo
  echo "## App settings to recreate in Coolify"
  echo "source:      https://github.com/h0lyvltron/school-nanny (branch main)"
  echo "build pack:  Dockerfile at repo root"
  echo "domain:      https://school-nanny.home"
  echo "internal port / Ports Exposes: 8080"
  echo "persistent storage: ${DATA_DIR} -> /data"
  echo "env: PORT=8080, BASE_URL=https://school-nanny.home"
  echo "container user: 65532:65532 (distroless nonroot)"
  echo
  echo "## TLS"
  echo "The PoC serves school-nanny.home with a mkcert certificate, which is"
  echo "temporary. Production should use a public CA (Let's Encrypt via Coolify)"
  echo "on a real hostname. No private keys are stored in this archive."
} > "${INFRA_STAGE}/notes.md"

INFRA_ARCHIVE="${INFRA_ARCHIVE_DIR}/school-nanny-infra-${STAMP}.tar.gz"
echo "==> Archiving host configuration..."
tar -czf "${INFRA_ARCHIVE}.partial" -C "${STAGE}" "school-nanny-infra-${STAMP}"
mv "${INFRA_ARCHIVE}.partial" "${INFRA_ARCHIVE}"
chmod 600 "${INFRA_ARCHIVE}"

# --- Prune ----------------------------------------------------------------
# Only runs after both archives landed, so a failing night never takes the last
# good copy with it.
echo "==> Removing archives older than ${KEEP_DAYS} days..."
find "${DATA_ARCHIVE_DIR}" -maxdepth 1 -name 'school-nanny-data-*.tar.gz' \
  -mtime "+${KEEP_DAYS}" -print -delete
find "${INFRA_ARCHIVE_DIR}" -maxdepth 1 -name 'school-nanny-infra-*.tar.gz' \
  -mtime "+${KEEP_DAYS}" -print -delete
find "${DATA_ARCHIVE_DIR}" "${INFRA_ARCHIVE_DIR}" -maxdepth 1 -name '*.partial' \
  -mtime +1 -delete 2>/dev/null || true

echo
echo "Done."
du -h "${DATA_ARCHIVE}" "${INFRA_ARCHIVE}"
echo
echo "Kept:"
ls -1 "${DATA_ARCHIVE_DIR}" | tail -n "$((KEEP_DAYS + 1))"
echo
echo "To restore, see scripts/RESTORE.md"
