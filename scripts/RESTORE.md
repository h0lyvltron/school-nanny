# Restoring School Nanny

Two different emergencies, two different answers.

- **The records are wrong or gone** but the machine is fine: restore a data archive.
- **The machine is gone**: rebuild the host from the scripts in this folder, then
  restore a data archive.

Everything the family actually owns lives in one folder, `/srv/school-nanny/data`
(`school.db`, `uploads/`, and the app's own `backups/`). The nightly job archives
that folder to `/srv/school-nanny/archives/data/` and keeps seven days.

## Restore the records

```bash
# 1. Stop the app so nothing writes while the folder is swapped.
docker ps -q --filter label=coolify.resourceName=school-nanny-local | xargs -r docker stop

# 2. Keep what is there now. Restoring the wrong archive should not be final.
sudo mv /srv/school-nanny/data "/srv/school-nanny/data.replaced-$(date +%Y%m%d-%H%M%S)"

# 3. Unpack the archive you want.
ls -la /srv/school-nanny/archives/data/
sudo tar -C /srv/school-nanny -xzf \
  /srv/school-nanny/archives/data/school-nanny-data-YYYYMMDD-HHMMSS.tar.gz

# 4. The container runs as distroless nonroot.
sudo chown -R 65532:65532 /srv/school-nanny/data

# 5. Start it again.
docker ps -aq --filter label=coolify.resourceName=school-nanny-local | xargs -r docker start
```

Then open <https://school-nanny.home> and check that the planner, a kid's
profile, and an attachment all load. Delete the `data.replaced-*` folder once
you are happy, not before.

## Rebuild the host

Order matters: DNS and Docker first, then Coolify, then the app, then the data.

1. **Docker at boot** — without this, nothing comes back after a reboot.

   ```bash
   sudo ./scripts/enable-coolify-on-boot.sh
   ```

2. **LAN DNS** — teaches the network what `school-nanny.home` means.

   ```bash
   sudo ./scripts/setup-lan-dns.sh
   ```

   This also installs the retry drop-in and the NetworkManager dispatcher hook.
   Both exist because dnsmasq starts before Wi-Fi has the static address and
   would otherwise fail permanently at boot.

3. **Firewall** — the LAN needs DNS, and Coolify's containers need to reach the
   host's SSH port or the dashboard reports the server as unreachable.

   ```bash
   sudo ufw allow from 192.168.1.0/24 to any port 53 proto udp comment 'school-nanny LAN DNS'
   sudo ufw allow from 192.168.1.0/24 to any port 53 proto tcp comment 'school-nanny LAN DNS'
   sudo ufw allow from 192.168.1.0/24 to any port 80 proto tcp comment 'school-nanny http'
   sudo ufw allow from 192.168.1.0/24 to any port 443 proto tcp comment 'school-nanny https'
   sudo ufw allow from 192.168.1.0/24 to any port 8000 proto tcp comment 'coolify UI'
   sudo ufw allow from 10.0.0.0/24 to any port 22 proto tcp comment 'coolify docker0 SSH'
   sudo ufw allow from 10.0.1.0/24 to any port 22 proto tcp comment 'coolify bridge SSH'
   ```

4. **Coolify** — see `./scripts/coolify-host-prep.sh` for the printed steps. On
   Omarchy the installer must be told it is Arch; `ID=omarchy` is not on its
   list.

   ```bash
   curl -fsSL https://cdn.coollabs.io/coolify/install.sh | sed 's/^OS_TYPE=.*/OS_TYPE=arch/' | sudo bash
   ```

   Coolify connects to this machine over SSH as `root`, so its key has to be in
   `/root/.ssh/authorized_keys` with `PermitRootLogin prohibit-password`.

5. **The app** — recreate it in Coolify, or apply the Ansible playbook
   (`ansible/`). The settings are also written into every infra archive
   (`notes.md`):

   | Setting | Value |
   | --- | --- |
   | Source | `https://github.com/h0lyvltron/school-nanny`, branch `main` |
   | Build pack | Dockerfile, repo root |
   | Domain | `https://school-nanny.home` |
   | Ports Exposes / internal port | `8080` |
   | Persistent storage | `/srv/school-nanny/data` → `/data` |
   | Environment | `PORT=8080`, `BASE_URL=https://school-nanny.home` |

   Port Mappings stays empty: publishing the port on the host would bypass the
   proxy that terminates TLS.

6. **The records** — restore an archive as above.

7. **The nightly backup** — a rebuilt host with no timer is the same mistake
   twice.

   ```bash
   sudo ./scripts/install-backup-timer.sh
   ```

## Certificates

The current certificate is a **mkcert** one, and it is temporary. It exists to
prove that HTTPS works end to end on `school-nanny.home`, which means every
device has to be told to trust a CA that no one else recognises. Nothing about
it is worth restoring from a backup, and its private keys are deliberately kept
out of the archives.

For the lab, reissue instead of restoring:

```bash
./scripts/setup-lan-certs.sh
sudo install -d -m 700 /data/coolify/proxy/certs
sudo cp ~/.local/share/school-nanny/certs/school-nanny.home.pem \
  /data/coolify/proxy/certs/school-nanny.home.cert
sudo cp ~/.local/share/school-nanny/certs/school-nanny.home-key.pem \
  /data/coolify/proxy/certs/school-nanny.home.key
sudo chmod 600 /data/coolify/proxy/certs/school-nanny.home.key
```

Then in Coolify: Servers → localhost → Proxy → Dynamic Configurations, a file
containing:

```yaml
tls:
  certificates:
    - certFile: /traefik/certs/school-nanny.home.cert
      keyFile: /traefik/certs/school-nanny.home.key
```

Every device that uses the app then has to install `rootCA.pem`. When the app
moves to a real hostname, drop all of this and let Coolify get a certificate
from a public authority; the only thing to carry forward is the hostname.

## What is in the infra archive

`/srv/school-nanny/archives/infra/` holds the fiddly host configuration: the
dnsmasq drop-in, the two boot-race fixes, the backup units, Coolify's
`/data/coolify/source/.env` (which contains its `APP_KEY`), and a `notes.md`
with the firewall rules and app settings as they were that night.

It is a reference to check the rebuild against, not something to unpack over a
running system. Restore Coolify's `.env` from it only if Coolify itself is
being recovered in place; the rest of the files are produced by the scripts.
