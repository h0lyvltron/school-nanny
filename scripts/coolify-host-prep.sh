#!/usr/bin/env bash
# Phase A: host prep notes for Coolify on this machine.
# Most steps need your password — run the printed block yourself.
set -euo pipefail

LAN_CIDR="${LAN_CIDR:-192.168.1.0/24}"
CERT_DIR="${CERT_DIR:-${HOME}/.local/share/school-nanny/certs}"

cat <<EOF
School Nanny — Coolify host prep
================================

1) Docker access (you are not in the docker group yet):
   sudo usermod -aG docker \$USER
   # then log out/in (or: newgrp docker)

2) Optional — trust mkcert CA in THIS Linux box's browsers:
   sudo env PATH="\$HOME/.local/bin:\$PATH" CAROOT="\$HOME/.local/share/mkcert" mkcert -install

3) Firewall (if ufw is active) — Coolify UI + HTTP(S) from LAN:
   sudo ufw allow from ${LAN_CIDR} to any port 8000 proto tcp comment 'coolify UI'
   sudo ufw allow from ${LAN_CIDR} to any port 80 proto tcp comment 'school-nanny http'
   sudo ufw allow from ${LAN_CIDR} to any port 443 proto tcp comment 'school-nanny https'

4) Install Coolify (creates /data/coolify; needs root).
   Omarchy reports ID=omarchy; Coolify only allowlists arch/manjaro/etc., so force OS_TYPE=arch:
   curl -fsSL https://cdn.coollabs.io/coolify/install.sh | sed 's/^OS_TYPE=.*/OS_TYPE=arch/' | sudo bash

5) Open http://192.168.1.181:8000 — create admin, then deploy Dockerfile from branch hosted.

6) Custom SSL for school-nanny.home (Coolify domain → custom cert):
   cert: ${CERT_DIR}/school-nanny.home.pem
   key:  ${CERT_DIR}/school-nanny.home-key.pem

7) Windows trust (C4): copy ${CERT_DIR}/rootCA.pem to her PC, then elevated:
   .\\scripts\\trust-mkcert-windows.ps1 -RootCAPath .\\rootCA.pem

Certs already on this machine under:
  ${CERT_DIR}
EOF
