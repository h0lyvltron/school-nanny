# Ryzen cutover (Phase E)

Move the always-on Coolify instance from this development machine to the spare
Ryzen 7 2700X. Same image, same data layout, same hostname.

## Before you start

1. Ryzen has Linux installed, a static/pinned LAN IP (or you will move
   `192.168.1.181` to it), Docker, and Coolify.
2. `ansible/inventory.ini` has the Ryzen host uncommented and reachable over SSH.
3. Nightly backups on the current host are healthy (`systemctl status school-nanny-backup.timer`).

## Cutover steps

```bash
# 1) On the current host — stop writes (Coolify → stop the app) then archive data.
sudo ./scripts/backup-school-nanny.sh
sudo tar -C /srv/school-nanny -czf /tmp/school-nanny-data-cutover.tgz data

# 2) Copy archive + checkout scripts to Ryzen.
scp /tmp/school-nanny-data-cutover.tgz ryzen:/tmp/
scp -r ansible scripts ryzen:~/school-nanny-ops/

# 3) On Ryzen — restore data and run Ansible (dns, docker, backups, firewall).
sudo mkdir -p /srv/school-nanny
sudo tar -C /srv/school-nanny -xzf /tmp/school-nanny-data-cutover.tgz
sudo chown -R 65532:65532 /srv/school-nanny/data

cd ~/school-nanny-ops/ansible
ansible-galaxy collection install -r requirements.yml
ansible-playbook -i inventory.ini site.yml -l ryzen --ask-become-pass \
  -e coolify_api_token=...   # when API is enabled

# 4) In Coolify on Ryzen: create (or upsert) the same Dockerfile app from main,
#    mount /srv/school-nanny/data → /data, set BASE_URL + optional hosted env
#    (see docs/HOSTED.md), attach the mkcert leaf (or share CAROOT).

# 5) Point LAN DNS at Ryzen (update dnsmasq address= or move 192.168.1.181).
#    Her Windows/iPad keep using school-nanny.home.

# 6) Smoke https://school-nanny.home then stop the app on the old host.
```

Helper: [`scripts/cutover-rsync-data.sh`](../scripts/cutover-rsync-data.sh) for a live
`rsync` of `/srv/school-nanny/data` when both machines are on the LAN.

## After cutover

- This development machine is build/dev only.
- Keep Ansible inventory `ryzen` as the hosted target.
- Renewals: same hostname; re-issue mkcert leaf on Ryzen or copy CAROOT.
