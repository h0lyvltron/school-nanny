# Ansible

Getting this machine back, or standing up the next one, from a checkout.

The shell scripts in [`../scripts`](../scripts) stay the source of truth for what
each piece of the host should look like. Ansible's job here is to decide *which*
of them run on *which* machine, in the right order, and to do the few things a
one-shot script cannot: copy files to a host you are not sitting at, and talk to
Coolify's API.

Roles that simply re-run a script are not a failure of the design. A script that
has already been debugged against a real Omarchy box is more trustworthy than a
fresh translation of it into YAML, and duplicating the logic would give the two
copies a chance to disagree.

## Running it

```bash
sudo pacman -S --needed ansible          # not installed by default

cd ansible
ansible-galaxy collection install -r requirements.yml
ansible-playbook -i inventory.ini site.yml --ask-become-pass
```

The distribution's `ansible` package already bundles `community.general`, so the
galaxy step is a no-op there. It matters if you installed `ansible-core` alone,
where the `pacman` and `ufw` modules are missing and the playbook will not even
parse.

Against this machine it is a local connection, so nothing needs SSH. Adding the
Ryzen box later means filling in its entry in [`inventory.ini`](inventory.ini)
and running the same playbook with `-l ryzen`. Cutover steps:
[`docs/RYZEN_CUTOVER.md`](../docs/RYZEN_CUTOVER.md). Public VPS notes:
[`docs/PUBLIC_VPS.md`](../docs/PUBLIC_VPS.md). Hosted app mode:
[`docs/HOSTED.md`](../docs/HOSTED.md).

Useful selections:

```bash
# just the backup timer
ansible-playbook -i inventory.ini site.yml --tags backups --ask-become-pass

# everything except the parts that reach the network
ansible-playbook -i inventory.ini site.yml --skip-tags coolify --ask-become-pass

# see what would change
ansible-playbook -i inventory.ini site.yml --check --diff --ask-become-pass
```

## What each role does

| Role | Tag | What it does |
| --- | --- | --- |
| `common` | always | Copies `scripts/` to `/usr/local/lib/school-nanny/scripts` so the rest can run them, on this host or a remote one |
| `docker` | `docker` | Installs Docker and enables it at boot, which is what brings Coolify and the app back after a reboot |
| `lan_dns` | `dns` | Runs `setup-lan-dns.sh`: the `school-nanny.home` record, plus the two fixes for dnsmasq losing a race with Wi-Fi |
| `firewall` | `firewall` | The LAN rules, and the Docker-bridge rules Coolify needs to SSH to its own host |
| `coolify` | `coolify` | Checks whether Coolify is installed and prints the install command if not. Deliberately does not install it unattended |
| `backups` | `backups` | Installs the nightly backup service and timer |
| `school_nanny_app` | `app` | Creates or updates the application in Coolify over its API, when a token is configured |

## Coolify's API

`school_nanny_app` is skipped unless `coolify_api_token` is set, because the API
is disabled on a fresh Coolify and the token has to be made by hand:
Settings, then Keys & Tokens.

Keep the token out of the repository. Either pass it for one run:

```bash
ansible-playbook -i inventory.ini site.yml --tags app \
  -e coolify_api_token="$(cat ~/.config/school-nanny/coolify-token)"
```

or keep it in an encrypted file:

```bash
ansible-vault create vault.yml        # coolify_api_token: <token>
ansible-playbook -i inventory.ini site.yml -e @vault.yml --ask-vault-pass --ask-become-pass
```

`vault.yml` is gitignored. Encrypted or not, a deploy token does not belong in
the history of a public repository.

## What is not here

Restoring the family's records. That is deliberate: it overwrites the only copy
of everything they have done, and it should be a decision someone makes while
looking at the archives, not a task that runs because it was in a playbook. The
steps are in [`../scripts/RESTORE.md`](../scripts/RESTORE.md).
