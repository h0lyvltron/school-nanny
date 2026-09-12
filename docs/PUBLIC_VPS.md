# Public VPS (optional)

Same container image and `/data` layout as home Coolify, on a small VPS, when
you prefer not to run a house server (or as a temporary off-site host).

## Requirements

- Linux VPS with Docker (Coolify recommended) or plain Compose
- Public DNS name → VPS (Let's Encrypt via Coolify)
- Copy of `DATA_ROOT` (`/srv/school-nanny/data` or Compose volume)

## Compose path

```bash
# On the VPS, with a checkout of main:
export SCHOOL_NANNY_MODE=hosted
export BASE_URL=https://app.example.com
export INVITE_CODE='…'   # set a real invite before opening signup
docker compose -f compose.hosted.yml up --build -d
```

`compose.hosted.yml` binds a named volume at `/data`. To import a home backup:

```bash
# Stop the stack, replace the volume contents with a restored tarball, chown 65532, start.
```

## Coolify path

Identical to the home app: Dockerfile from `main`, persistent storage → `/data`,
env from [`docs/HOSTED.md`](HOSTED.md), domain = public hostname.

## Cutover from home

1. Stop home app writes; run `scripts/backup-school-nanny.sh`.
2. Copy `/srv/school-nanny/data` (or the archive) to the VPS data dir.
3. Deploy; switch public DNS; smoke `/healthz` and login.
4. Leave home Coolify stopped or remove the LAN domain from the old host.
