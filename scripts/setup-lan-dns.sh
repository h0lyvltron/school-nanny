#!/usr/bin/env bash
# Track 1 LAN DNS: configure dnsmasq for school-nanny.home
# Upstream: Starlink gateway 192.168.1.1
# Listen: 192.168.1.181 (and 127.0.0.1)
#
# Run: sudo ./scripts/setup-lan-dns.sh
set -euo pipefail

LAN_IP="${LAN_IP:-192.168.1.181}"
HOSTNAME_LAN="${HOSTNAME_LAN:-school-nanny.home}"
UPSTREAM_DNS="${UPSTREAM_DNS:-192.168.1.1}"
CONF_DIR=/etc/dnsmasq.d
CONF_FILE="${CONF_DIR}/school-nanny.conf"

if [[ "$(id -u)" -ne 0 ]]; then
  echo "Run as root: sudo $0" >&2
  exit 1
fi

echo "==> Ensuring packages..."
pacman -S --noconfirm --needed dnsmasq bind

echo "==> Ensuring ${CONF_DIR} and conf-dir..."
mkdir -p "${CONF_DIR}"
if [[ -f /etc/dnsmasq.conf ]]; then
  if ! grep -qE '^[[:space:]]*conf-dir=/etc/dnsmasq.d/,\*\.conf' /etc/dnsmasq.conf; then
    if grep -qE '^#conf-dir=/etc/dnsmasq.d/,\*\.conf' /etc/dnsmasq.conf; then
      sed -i 's|^#conf-dir=/etc/dnsmasq.d/,*.conf|conf-dir=/etc/dnsmasq.d/,*.conf|' /etc/dnsmasq.conf
    else
      printf '\nconf-dir=/etc/dnsmasq.d/,*.conf\n' >> /etc/dnsmasq.conf
    fi
  fi
fi

echo "==> Writing ${CONF_FILE}..."
cat > "${CONF_FILE}" <<EOF
# School Nanny LAN DNS (managed by scripts/setup-lan-dns.sh)
address=/${HOSTNAME_LAN}/${LAN_IP}

# LAN only — avoid 127.0.0.1 so we do not fight systemd-resolved on the stub.
listen-address=${LAN_IP}
bind-interfaces
port=53

no-resolv
server=${UPSTREAM_DNS}

domain-needed
bogus-priv
EOF

# Do not add After=network-online.target — on some hosts it creates an ordering cycle with NM.
# Instead: retry on failure + NM dispatcher (Wi-Fi often comes up after dnsmasq first starts).
rm -f /etc/systemd/system/dnsmasq.service.d/wait-network.conf
mkdir -p /etc/systemd/system/dnsmasq.service.d
cat > /etc/systemd/system/dnsmasq.service.d/retry-on-boot.conf <<'EOF'
[Unit]
StartLimitIntervalSec=0

[Service]
Restart=on-failure
RestartSec=5
EOF

cat > /etc/NetworkManager/dispatcher.d/99-school-nanny-dnsmasq <<EOF
#!/bin/bash
IFACE="\$1"
STATUS="\$2"
case "\$STATUS" in
  up|dhcp4-change|connectivity-change) ;;
  *) exit 0 ;;
esac
if ip -4 -o addr show dev "\$IFACE" 2>/dev/null | grep -q " ${LAN_IP}/"; then
  systemctl reset-failed dnsmasq.service 2>/dev/null || true
  systemctl restart dnsmasq.service 2>/dev/null || true
fi
EOF
chmod 755 /etc/NetworkManager/dispatcher.d/99-school-nanny-dnsmasq

systemctl daemon-reload

echo "==> Port 53 on ${LAN_IP}..."
ss -ulnp | grep "${LAN_IP}:53" || echo "(nothing on ${LAN_IP}:53 yet — good)"

echo "==> Enabling dnsmasq..."
dnsmasq --test
systemctl reset-failed dnsmasq 2>/dev/null || true
systemctl enable --now dnsmasq
systemctl restart dnsmasq

echo "==> Verifying..."
systemctl --no-pager --full status dnsmasq | head -n 20
echo
echo -n "${HOSTNAME_LAN}: "
dig @"${LAN_IP}" "${HOSTNAME_LAN}" +short
echo -n "example.com: "
dig @"${LAN_IP}" example.com +short | head -n 3

echo
echo "If UFW is active, allow LAN DNS (idempotent-ish):"
echo "  sudo ufw allow from 192.168.1.0/24 to any port 53 proto udp comment 'school-nanny LAN DNS'"
echo "  sudo ufw allow from 192.168.1.0/24 to any port 53 proto tcp comment 'school-nanny LAN DNS'"
echo
echo "Done. Next (on her devices): set DNS for home Wi-Fi to ${LAN_IP}"
echo "  Windows: Settings → Wi-Fi → [network] → DNS manual → ${LAN_IP}"
echo "  iPad:    Settings → Wi-Fi → (i) → Configure DNS → Manual → ${LAN_IP}"
echo "Then: nslookup ${HOSTNAME_LAN}"
