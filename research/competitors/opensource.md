# Open-source peers (GitHub / self-hosted)

Date: 2026-09-13. Method: web search for public repos; skim README feature
lists and license notes. This is **architecture/feature brainstorming**, not a
code audit or license-compliance review for shipping their code.

We look at OSS peers because READMEs and schemas often reveal **how** they
model multi-user access, attendance, grading, and backup—useful for our own
bespoke design. We do **not** copy their code or UI.

## Leads

| Project | Repo | Stack (from README) | License (claimed) | Why it’s relevant |
| --- | --- | --- | --- | --- |
| **Homeschool** (Matt Layman) | https://github.com/mblayman/homeschool | Django; hosted product with open code | MIT | Closest “homeschool planning app” with a mature public codebase (~240★); real-world planner domain model |
| **OurSchool** | https://github.com/DGAzr/ourschool (also forks e.g. abdasis/ourschool) | FastAPI + PostgreSQL + Node; Docker | AGPL-3.0 | Self-hosted admin grind: attendance, subjects, assignments, grading, reports; **parent + student logins**; backup/restore; REST/API keys |
| **Homeschool Hero** | https://github.com/x3nc0n/homeschool-hero | FastAPI + React + PostgreSQL; Docker | (check repo) | Explicit **multi-family tenancy** with owner / parent / co-parent / tutor / student memberships; OIDC/SAML optional; transcripts, attendance, audit logs |
| **HomeLearnAI** | https://github.com/buger/homelearnai | Laravel + PostgreSQL + HTMX | MIT | Curriculum hierarchy (subject→unit→topic→session), spaced repetition, ICS import, multi-child; open-source + self-host positioning |
| **Homestead** | https://github.com/tbh-23/Homestead | Static JS + Puter.js backend | MIT (app); curriculum data separate | Mastery-based path + adaptive daily calendar; interesting curriculum-as-graph idea (Marble taxonomy)—less of a family planner clone |
| **Homeschool Offline** | https://github.com/corpora-inc/encorpora (homeschool-offline) | Tauri + React + SQLite | (check repo) | Offline-first calendar/notes/photos; reinforces local-data philosophy |
| **homeschool** (CooperandGoodman) | https://git.chns.tech/CooperandGoodman/homeschool | Vue + FastAPI + MySQL; WebSockets | (check forge) | Schedule templates, live timers, TV dashboard—day-of execution UX |

## What stood out for *our* design

### Tenancy / auth (feeds AUTH_ROADMAP)

- **Homeschool Hero** advertises multi-family tenancy and roles (owner, parent, co-parent, tutor, student)—closest conceptual match to our account + membership + RBAC sketch.
- **OurSchool** separates **parent/admin** vs **student** logins; students see their own work. Aligns with our kid PIN / scoped session idea.
- Neither SaaS marketing page is as explicit about self-hosted RBAC as these READMEs.

### Records & compliance

- OurSchool / Homeschool Hero lean hard into attendance, gradebook, report cards, transcripts, backup/restore—validates those as near-term competitive gaps for us.
- OurSchool’s merge vs wipe-and-restore backup modes are worth remembering when we polish family export/import.

### Curriculum / learning

- HomeLearnAI: hierarchical curriculum + spaced repetition + ICS—different product center (learning engine vs week planner), but hierarchy and external calendar import are idea fodder.
- Homestead: prerequisite graph / mastery gating—optional future “curriculum intelligence,” not core planner parity.
- Matt Layman’s Homeschool: classic planning app; worth a deeper read of models when we implement TOC/YAML and school-year planning.

### Philosophy overlap with School Nanny

| Theme | OSS peers | Us |
| --- | --- | --- |
| Self-host / own your data | OurSchool, Hero, HomeLearnAI, Offline | Have (local + Coolify) |
| Multi-principal family access | Hero, OurSchool | Designed (PIN + RBAC); not built yet |
| Open interchange / export | OurSchool JSON import/export, Offline ZIP | Have zip; curriculum YAML planned |
| AI grading / spaced repetition | Hero (optional Ollama), HomeLearnAI | Later/maybe—not core |

## Ideas to pull into our backlog (bespoke)

Already partly in [`ideas.md`](ideas.md); OSS review reinforces:

1. **Membership roles** as first-class (Hero) — stick with owner / co_parent / teacher / caregiver / kid + capability flags.
2. **Student-scoped UI** (OurSchool) — kid PIN → Today/schedule only.
3. **Serious backup semantics** (OurSchool) — clear merge vs replace on family import.
4. **School-year / term reporting** (OurSchool, Hero) — package attendance + grades into transcripts.
5. **Optional ICS in** (HomeLearnAI) — adult/family calendar sync later.
6. **Do not** chase mastery graphs or social networks unless users ask.

## License caution

AGPL (OurSchool) and mixed data licenses (Homestead/Marble) mean we **read for ideas**, not vendor their code into School Nanny. Prefer MIT/BSD patterns only if we ever intentionally vendor a small library after legal/product review.
