#!/usr/bin/env bash
# Install the nightly backup as a systemd service and timer.
#
# The backup script is copied to /usr/local/sbin rather than run from the repo:
# a timer that points at a working copy breaks the night someone moves, renames,
# or reinstalls the checkout. Re-run this script after changing the backup
# script to publish the new version.
#
# Run: sudo ./scripts/install-backup-timer.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SOURCE_SCRIPT="${SCRIPT_DIR}/backup-school-nanny.sh"
INSTALLED_SCRIPT="/usr/local/sbin/school-nanny-backup"
UNIT_DIR=/etc/systemd/system
DATA_DIR="${DATA_DIR:-/srv/school-nanny/data}"
ARCHIVE_ROOT="${ARCHIVE_ROOT:-/srv/school-nanny/archives}"
KEEP_DAYS="${KEEP_DAYS:-7}"
APP_LABEL="${APP_LABEL:-coolify.resourceName=school-nanny-local}"
ON_CALENDAR="${ON_CALENDAR:-*-*-* 03:00:00}"

if [[ "$(id -u)" -ne 0 ]]; then
  echo "Run as root: sudo $0" >&2
  exit 1
fi

if [[ ! -f "${SOURCE_SCRIPT}" ]]; then
  echo "Cannot find ${SOURCE_SCRIPT}" >&2
  exit 1
fi

echo "==> Installing ${INSTALLED_SCRIPT}..."
install -m 700 "${SOURCE_SCRIPT}" "${INSTALLED_SCRIPT}"

echo "==> Writing units..."
cat > "${UNIT_DIR}/school-nanny-backup.service" <<EOF
[Unit]
Description=School Nanny nightly backup
# The backup stops the app container for a few seconds, so it needs Docker up.
Requires=docker.service
After=docker.service

[Service]
Type=oneshot
Environment=DATA_DIR=${DATA_DIR}
Environment=ARCHIVE_ROOT=${ARCHIVE_ROOT}
Environment=KEEP_DAYS=${KEEP_DAYS}
Environment=APP_LABEL=${APP_LABEL}
ExecStart=${INSTALLED_SCRIPT}
EOF

cat > "${UNIT_DIR}/school-nanny-backup.timer" <<EOF
[Unit]
Description=Run the School Nanny backup nightly

[Timer]
OnCalendar=${ON_CALENDAR}
# This is a desktop that is not always awake at 3am; catch up on the next boot
# rather than silently skipping a day.
Persistent=true
RandomizedDelaySec=5m

[Install]
WantedBy=timers.target
EOF

systemctl daemon-reload
systemctl enable --now school-nanny-backup.timer

echo
echo "==> Timer:"
systemctl list-timers school-nanny-backup.timer --no-pager || true

echo
echo "Installed. Useful commands:"
echo "  sudo systemctl start school-nanny-backup.service   # run one now"
echo "  journalctl -u school-nanny-backup.service -n 50    # see the last run"
echo "  ls -la ${ARCHIVE_ROOT}/data                        # the archives"
