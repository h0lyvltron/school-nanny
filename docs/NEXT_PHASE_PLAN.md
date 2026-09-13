---
name: Next phase enhancements
overview: "Adult-neutral copy (no Mom/her defaults), design-only auth/email/OAuth, curriculum TOC→YAML in-app with YAML export and richer family archives, and a search-led competitor feature matrix for brainstorming our own roadmap."
todos:
  - id: adult-neutral-copy
    content: "Adult-neutral copy: replace Mom/her defaults and UI/docs/tests with Parent/neutral adult language"
    status: pending
  - id: auth-roadmap-doc
    content: "Write docs/AUTH_ROADMAP.md — design only: RBAC multi-principal family tenancy, password/confirm/reset/OAuth/mailer"
    status: completed
  - id: toc-in-app
    content: Go-port toc2yaml parser + Curriculum paste UI + POST /curriculum/from-toc
    status: pending
  - id: curriculum-yaml-export
    content: Add per-plan (and optional all) YAML download round-tripping import schema
    status: pending
  - id: export-include-curriculum-yaml
    content: Extend family zip with curriculum/*.yaml; document DB-authoritative restore
    status: pending
  - id: competitor-matrix
    content: Feature matrix via search leads + OSS GitHub peers (manual review; no crawling)
    status: completed
isProject: false
---

# Next phase: adult-neutral copy, auth design, curriculum tools, feature brainstorm

## Decisions locked

- **Copy:** call this workstream **adult-neutral**, not “inclusive.” Goal is simply not assuming the adult is Mom / she / her. Avoid seeding broader “inclusivity” framing in product language.
- **Auth:** design-only this phase. Control plane must be shaped for **multiple principals per family with RBAC** (owner parent, secondary adults, kid-scoped logins)—not a single household password forever. Mailer/confirm/reset/OAuth remain design discussion; implement after lock-in.
- **Competitor research:** search for commercial leads *and* public GitHub/self-hosted repos; manual matrix. No crawling competitor sites; read OSS for ideas only (no vendoring their app code).

## Workstream 1 — Adult-neutral verbiage (ship early)

Default adult and UI copy assume “Mom” / “her.”


| Location | Today | Change |
| --- | --- | --- |
| Default adult seed | name/role `"Mom"` | **"Parent"** / **"Parent"** (editable in Settings) |
| Adult schedule heading | “Her week” | **“{Name}'s week”** |
| Settings / README / comments / tests | Mom / her | Parent / neutral adult wording |

Do **not** rename the holiday “Mother’s Day.”

Acceptance: new empty family shows “Parent”; no Mom-as-default assumption in UI/docs/tests.

## Workstream 2 — Auth design (discussion artifact)

Canonical write-up: [`docs/AUTH_ROADMAP.md`](AUTH_ROADMAP.md).

**Tenancy shape (required direction):**

- **Family** = tenant / data boundary (unchanged).
- **Account** = credentials (email/password, later OAuth)—not “the Mom user.”
- **Membership** = account ↔ family + **role** + optional **kid scope**.
- **Roles (v1 vocabulary):** `owner` (primary admin parent) · `co_parent` · `teacher` · `caregiver` · `kid` (own schedule only).
- Owner is **email+password**; secondary adults and kids are **family-issued username+PIN** by default (owner resets/issues; optional `can_manage_kid_logins` for kid PINs only).
- Secondary adults do **not** get sensitive tenant Settings; owner remains break-glass for household auth.
- Deploy `INVITE_CODE` = create-family gate; household PIN issuance is per-family, owner-managed.
- Schema sketch: `accounts` + `memberships` + `pin_credentials` so multi-adult and kid logins do not require a rewrite later.

Also covered in the roadmap: credential options A–D, capability matrix, owner password/HIBP/OAuth/mailer notes, migration from one-user-per-family.

## Workstream 3 — Curriculum TOC + YAML export + archives

In-app TOC → plan (Go port of toc2yaml), YAML download, family zip includes `curriculum/*.yaml`.

## Workstream 4 — Competitor matrix (done; includes OSS)

See [`research/competitors/`](../research/competitors/), including
[`opensource.md`](../research/competitors/opensource.md) for GitHub/self-hosted peers.

## Sequencing

1. Competitor research (done)
2. Adult-neutral copy
3. Curriculum TOC/YAML/export
4. Auth roadmap doc (done as design; implement later)

## After this phase

Research is folded into concrete product features in
[`PRODUCT_ROADMAP.md`](PRODUCT_ROADMAP.md) (Waves 0–4: bump/print → PIN household →
records/transcripts → later polish). This file remains the **immediate** slice
(Wave 0); do not expand it into the full backlog.
