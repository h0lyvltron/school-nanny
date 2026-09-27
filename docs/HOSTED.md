# Hosted mode (Coolify)

Single-tenant Phase A stays the default: omit `SCHOOL_NANNY_MODE` and the
container keeps using `/data/school.db` exactly as today.

## Enabling multi-tenant hosted mode

Set these on the Coolify application (Environment Variables):

| Variable | Example | Notes |
| --- | --- | --- |
| `SCHOOL_NANNY_MODE` | `hosted` | Turns on control DB + signup/login |
| `BASE_URL` | `https://school-nanny.home` | Also drives `Secure` cookies when `https://` |
| `INVITE_CODE` | (secret string) | Required to create a family. Signup stays closed when this is empty |
| `ALLOW_OPEN_SIGNUP` | `1` | Opens signup with no invite. The process logs that choice. Leave unset for invite-only |
| `SMTP_HOST` | `smtp.example.com` | Sends confirm, reset, and sign-in messages. Leave unset to skip mail |
| `SMTP_PORT` | `587` | Defaults to 587 |
| `SMTP_USERNAME` | | Optional SMTP auth |
| `SMTP_PASSWORD` | | Keep this in Coolify's env, not in git |
| `MAIL_FROM` | `nanny@example.com` | From address. Defaults to `school-nanny@SMTP_HOST` |
| `MAIL_LOG` | `1` | Prints messages on the server. For a private LAN only. Not the default |
| `PORT` | `8080` | Already set by Coolify |
| `COOKIE_SECURE` | `true` / `false` | Optional override; default follows `BASE_URL` |

Data layout after the first signup:

```text
/data/
  control.db
  families/{family_id}/
    school.db
    uploads/
    backups/
```

If `/data/school.db` still exists from Phase A, the **first** successful signup
moves it (and `uploads/` / `backups/` when present) into that family's folder.

## Health check

`GET /healthz` returns `ok`. Point Coolify's health check there.

## Hardening checklist (this machine)

- [x] Persistent `/srv/school-nanny/data` bind mount
- [x] Nightly host backups (`scripts/backup-school-nanny.sh` + timer)
- [x] LAN DNS + mkcert for `school-nanny.home`
- [x] `/healthz`
- [ ] Flip `SCHOOL_NANNY_MODE=hosted` only when ready for account signup (or after
      inviting the household with `INVITE_CODE`)
- [ ] Confirm two test accounts cannot see each other's kids (Settings)
- [ ] Keep Coolify UI (`:8000`) LAN-only

## Family export / import

Settings → **Family export / import** downloads or restores `school.db` +
`uploads/` for the signed-in family only. Host nightly tarballs remain ops.
