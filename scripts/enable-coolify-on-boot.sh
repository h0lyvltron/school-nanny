#!/usr/bin/env bash
# Enable Docker (and thus Coolify + school-nanny) to start at boot.
# Coolify containers already use restart: always/unless-stopped; they only
# come back when dockerd is running.
#
# Run: sudo ./scripts/enable-coolify-on-boot.sh
set -euo pipefail

if [[ "$(id -u)" -ne 0 ]]; then
  echo "Run as root: sudo $0" >&2
  exit 1
fi

echo "==> Enabling Docker at boot..."
systemctl enable --now docker.service
systemctl enable --now containerd.service 2>/dev/null || true

echo "==> Waiting for Docker..."
for _ in $(seq 1 30); do
  if docker info >/dev/null 2>&1; then
    break
  fi
  sleep 1
done
docker info >/dev/null

echo "==> Container restart policies (Coolify-managed):"
docker ps -a --filter label=coolify.managed=true \
  --format 'table {{.Names}}\t{{.Status}}\t{{.Label "coolify.resourceName"}}' || true

echo
echo "Docker is enabled. After reboot, dockerd starts and Coolify/app containers"
echo "with restart policies should come up on their own."
echo
echo "Manual check:"
echo "  systemctl status docker"
echo "  docker ps --filter label=coolify.managed=true"
echo
echo "Optional — start school-nanny if it was stopped (not removed):"
echo "  docker ps -aq --filter label=coolify.resourceName=school-nanny-local | xargs -r docker start"
