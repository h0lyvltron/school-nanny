#!/usr/bin/env bash
# Phase A: host prep notes for Coolify on this machine.
# Most steps need your password — run the printed block yourself.
set -euo pipefail

LAN_CIDR="${LAN_CIDR:-192.168.1.0/24}"
CERT_DIR="${CERT_DIR:-${HOME}/.local/share/school-nanny/certs}"
DATA_DIR="${DATA_DIR:-/srv/school-nanny/data}"

cat <<EOF
School Nanny — Coolify host prep
================================

1) Docker access and Docker at boot:
   sudo usermod -aG docker \$USER          # then log out/in (or: newgrp docker)
   sudo ./scripts/enable-coolify-on-boot.sh

2) LAN name for her devices:
   sudo ./scripts/setup-lan-dns.sh

3) Firewall. The first four rules are for the people using the app. The last two
   are for Coolify itself: it reaches this host over SSH *from a container*, so
   allowing only the LAN leaves the dashboard reporting "server is not reachable".
   sudo ufw allow from ${LAN_CIDR} to any port 8000 proto tcp comment 'coolify UI'
   sudo ufw allow from ${LAN_CIDR} to any port 80 proto tcp comment 'school-nanny http'
   sudo ufw allow from ${LAN_CIDR} to any port 443 proto tcp comment 'school-nanny https'
   sudo ufw allow from ${LAN_CIDR} to any port 22 proto tcp comment 'lan ssh'
   sudo ufw allow from 10.0.0.0/24 to any port 22 proto tcp comment 'coolify docker0 SSH'
   sudo ufw allow from 10.0.1.0/24 to any port 22 proto tcp comment 'coolify bridge SSH'

4) Install Coolify (creates /data/coolify; needs root).
   Omarchy reports ID=omarchy; Coolify only allowlists arch/manjaro/etc., so force OS_TYPE=arch:
   curl -fsSL https://cdn.coollabs.io/coolify/install.sh | sed 's/^OS_TYPE=.*/OS_TYPE=arch/' | sudo bash

5) Let Coolify log in as root over SSH (it manages even "localhost" that way).
   Paste the public key the dashboard shows you:
   sudo install -d -m 700 /root/.ssh
   echo 'ssh-ed25519 AAAA...' | sudo tee -a /root/.ssh/authorized_keys >/dev/null
   sudo chmod 600 /root/.ssh/authorized_keys
   printf 'PubkeyAuthentication yes\\nPermitRootLogin prohibit-password\\n' | sudo tee /etc/ssh/sshd_config.d/99-coolify.conf
   sudo systemctl reload sshd

6) Open http://192.168.1.181:8000 — create admin, choose "This Machine", then add
   the app from https://github.com/h0lyvltron/school-nanny (branch main):
     build pack        Dockerfile at repo root
     domain            https://school-nanny.home
     internal port     8080        (Ports Exposes; leave Port Mappings empty)
     persistent volume ${DATA_DIR} -> /data
     environment       PORT=8080, BASE_URL=https://school-nanny.home
   The folder is read and written by the container's nonroot user:
   sudo install -d -o 65532 -g 65532 ${DATA_DIR}

7) Certificate for school-nanny.home. This is a lab certificate and a temporary
   one — the real deployment should get a certificate from a public authority on
   a real hostname. For now:
   ./scripts/setup-lan-certs.sh
   sudo install -d -m 700 /data/coolify/proxy/certs
   sudo cp ${CERT_DIR}/school-nanny.home.pem /data/coolify/proxy/certs/school-nanny.home.cert
   sudo cp ${CERT_DIR}/school-nanny.home-key.pem /data/coolify/proxy/certs/school-nanny.home.key
   sudo chmod 600 /data/coolify/proxy/certs/school-nanny.home.key
   Then Coolify → Servers → localhost → Proxy → Dynamic Configurations, add:
     tls:
       certificates:
         - certFile: /traefik/certs/school-nanny.home.cert
           keyFile: /traefik/certs/school-nanny.home.key
   Each device then installs ${CERT_DIR}/rootCA.pem to trust it. Windows:
   .\\scripts\\trust-mkcert-windows.ps1 -RootCAPath .\\rootCA.pem

8) Nightly backups. A host with no timer is one bad day from losing the year.
   sudo ./scripts/install-backup-timer.sh

Recovering a broken or rebuilt machine: scripts/RESTORE.md
EOF
