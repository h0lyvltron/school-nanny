#!/usr/bin/env bash
# Recover LAN DNS when school-nanny.home stops resolving — usually because
# dnsmasq tried to bind the static address before Wi-Fi had it, failed five
# times, and systemd gave up on it.
#
# The configuration itself, including the retry drop-in and the NetworkManager
# hook that prevent that race, is written by setup-lan-dns.sh. This script only
# re-applies it and clears the failed state, so there is one definition of what
# the host should look like rather than two that can drift apart.
#
# Run: sudo ./scripts/fix-lan-dns.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LAN_IP="${LAN_IP:-192.168.1.181}"
HOSTNAME_LAN="${HOSTNAME_LAN:-school-nanny.home}"

if [[ "$(id -u)" -ne 0 ]]; then
  echo "Run as root: sudo $0" >&2
  exit 1
fi

if ! ip -4 -o addr show 2>/dev/null | grep -q " ${LAN_IP}/"; then
  echo "Warning: ${LAN_IP} is not on any interface yet." >&2
  echo "dnsmasq cannot bind it until the network is up; check nmcli first." >&2
fi

echo "==> Re-applying DNS configuration..."
LAN_IP="${LAN_IP}" HOSTNAME_LAN="${HOSTNAME_LAN}" "${SCRIPT_DIR}/setup-lan-dns.sh"

echo
echo "==> Checking from this machine..."
echo -n "${HOSTNAME_LAN}: "
dig @"${LAN_IP}" "${HOSTNAME_LAN}" +short +time=2 +tries=2 || true

echo
echo "If her devices still time out, confirm the firewall allows LAN DNS:"
echo "  sudo ufw status numbered | grep 53"
