# Product roadmap — research → School Nanny features

Status: planning artifact (2026-09-13). Turns competitor matrix, OSS peer
notes, WTM forum temperature, and the current next-phase work into **concrete
behaviors** we can build.

Sources: [`research/competitors/`](../research/competitors/) (especially
[`user-temperature.md`](../research/competitors/user-temperature.md),
[`matrix.md`](../research/competitors/matrix.md),
[`ideas.md`](../research/competitors/ideas.md)).
Auth design: [`AUTH_ROADMAP.md`](AUTH_ROADMAP.md).
Immediate slice: [`NEXT_PHASE_PLAN.md`](NEXT_PHASE_PLAN.md).

## Product thesis (from the research)

Parents are **pragmatic and slightly weary**. They do not want more menu items.
They want a tool that:

1. **Survives a messy week** (sick days, field trips, “we didn’t finish”).
2. Still produces **credible records** when oversight / high school appears.
3. Keeps **family-owned data** (local + optional hosted) without a second job of
   setup or a SaaS tax for notebook work.
4. Supports **hybrid paper + digital** (fridge/binder lists) and **more than one
   adult running the day**.

School Nanny already wins on planner depth (Today, week drag/copy, series,
subjects, files, adult calendar kept out of kid progress) and self-host /
family export. The roadmap closes the gaps that both SaaS marketing and forum
voice keep repeating—without chasing marketplace, social, budget, or mastery-engine
product centers.

## Already strong (protect; don’t dilute)

Code inventory (2026-09-13) matches this table; partials are called out in the waves below.

| Behavior | Notes |
| --- | --- |
| Multi-child Today + week planner | Table stakes; keep fast |
| Drag move / Ctrl-copy / copy-to-child chips | Differentiator—keep polishing |
| Recurring series + assignment push/pull | Foundation for bump/reschedule (W1.1 extends with policies) |
| Lesson **minutes** + progress hour labels | Foundation for hours reports (W3.1 aggregates / surfaces) |
| Lesson files + tests/scores | Records spine; not yet course GPA / transcripts |
| Attendance present/absent/excused + archive totals | Have; hours packaging and transcript still Wave 3 |
| Curriculum plans + YAML/CSV **import** + archive→plan | Have; TOC-in-app + YAML **download** still Wave 0 |
| Adult pinboard + adult week (off kid progress) | Distinctive; do not merge into “life OS” |
| Family zip export + local/LAN + hosted Coolify | Moat vs cloud-only planners |
| Invite-gated family create | Fits private deploy story |

## Explicitly out of scope (research → `no` / `skip`)

| Temptation | Why skip |
| --- | --- |
| Paid curriculum marketplace | Conflicts with open YAML + self-host; TOC→YAML instead |
| Social / community network | Out of philosophy |
| Family budget / shopping / chores as core | Life-OS creep (Planet/Panda) |
| Spaced-repetition / mastery graph | Different product center (HomeLearnAI/Homestead) |
| State “compliance packs” as legal advice | High maintenance; optional export *templates* later only |
| Native apps before print + PIN + reschedule | Responsive web first |

---

## Wave 0 — Finish the locked next phase

Ship what [`NEXT_PHASE_PLAN.md`](NEXT_PHASE_PLAN.md) already committed. Unblocks
later waves (curriculum portability, honest adult language, auth implementability).

| ID | Feature | Tangible behavior | Acceptance |
| --- | --- | --- | --- |
| **W0.1** | Adult-neutral copy | Default adult is **Parent**; schedule heading **“{Name}'s week”**; no Mom/she/her defaults in UI/docs/tests | New empty family shows Parent; Mother’s Day holiday unchanged |
| **W0.2** | TOC → curriculum in-app | Paste a book TOC; get a curriculum plan (Go port of toc2yaml) | UI + `POST /curriculum/from-toc`; plan editable like YAML import |
| **W0.3** | Curriculum YAML download | Per-plan (and optional all) download matching import schema | Round-trip: export → import without data loss of titles/order |
| **W0.4** | Family zip includes curriculum YAML | Archive has `curriculum/*.yaml`; restore UI explains merge vs replace in plain language | Zip contains YAML mirrors; Settings never shows server paths |
| **W0.5** | Auth design (done) | [`AUTH_ROADMAP.md`](AUTH_ROADMAP.md) locked enough to implement later | Roles + PIN model written; open policy questions listed |

**Research link:** DIY curriculum / TOC scheduling; setup anxiety; “share plans without marketplace.”

---

## Wave 1 — Survive the messy week (highest forum temperature)

Status: **implemented** on branch work after Wave 0 (Double up / Shift / Drop,
vacation shift, print CSS, setup checklist).

Highest emotional parity with Planet/Scholaric/Syllabird *and* the #1 WTM ask
(“bump two ways”). Builds on series we already have.

### W1.1 — Miss-day / bump with policy picker

**Problem:** Life happens; tools that only delete or leave orphans lose trust.

**Foundation we already have:** manual drag move, skip status, and assignment
**push/pull** along a series. Wave 1 adds an explicit **policy picker** and
vacation-range shift—not a greenfield scheduler.

**Behavior:**

- From Today, a lesson, or a day header: actions for **missed / skip / didn’t finish**.
- User picks a **policy** (remember last choice per kid or family preference):
  - **Double-up** — pile unfinished onto the next school day.
  - **Shift forward** — shove this lesson (or the rest of its series from here) one school day later; extend the end.
  - **Drop** — mark skipped / incomplete without moving anything (still logged).
- Vacation / “no school” range: shift scheduled schoolwork forward across the gap (same shift-forward engine).
- Never silently mutate; show a short preview (“3 lessons move to Thu–Mon”).

**Acceptance:** One missed Math lesson can double-up *or* shift the series; a
marked vacation week moves work after it; series integrity preserved for copies
(standalone copies stay standalone).

**Research:** WTM “bump two ways”; Planet/Syllabird/Panda auto-reschedule marketing.

### W1.2 — Printable Today & week lists

**Problem:** Digital brain, paper hands—fridge, binder, grandma’s clipboard.

**Behavior:**

- **Print Today** (all kids or one kid) — clean list: child, subject, title, checkbox-looking rows, optional notes line.
- **Print this week** per kid (and optional household sheet).
- CSS `@media print` first; PDF download if print CSS is not enough for tablets.
- No marketing chrome; high contrast; B&W-friendly.

**Acceptance:** From Today and kid/week views, one click yields a usable printed
checklist parents would put on the fridge.

**Research:** WTM print-layout threads; Planet weekly assignment sheets.

### W1.3 — Onboarding that doesn’t “pull hair out”

**Problem:** Tracker-class power with Skedtrack-class setup tax.

**Behavior:**

- First-run / empty family: short checklist — school year dates → kids → subjects to show → optional first curriculum paste.
- Sensible defaults already exist; this is guidance, not a wizard novel.

**Acceptance:** New family can plan a week in under a few minutes without reading the README.

**Research:** Setup anxiety vs Planet “easy” praise; price/value sensitivity (self-host is our answer to price).

---

## Wave 2 — Household principals (auth implement + day handoff)

**Status: implemented on `main`.** Hosted multi-principal RBAC: owner email+password,
family slug + username/PIN for co_parent / teacher / caregiver / kid; soft revoke;
owner-only export/import/backups; kid Today scoped to `kid_id`.

### W2.1 — Owner email + family PIN principals

| Done | Behavior |
| --- | --- |
| Control schema | `accounts`, `memberships`, `pin_credentials`; `families.slug`; migrate from `users` |
| Login | Email path + PIN path (family code / username / PIN) |
| Settings | Owner household logins panel: create / reset / revoke |

### W2.2 — Kid check-off surface

| Done | Behavior |
| --- | --- |
| Scope | Kid session sees own Today / Week / kid page only |
| Nav | No Settings, Curriculum, Attendance, Archive |

### W2.3 — Caregiver / co_parent day-run mode

| Done | Behavior |
| --- | --- |
| Caregiver | Today + Week; no Settings / Curriculum / Attendance / Archive |
| Co-parent / teacher | Day planning + Settings (kids/subjects); no sensitive export/backups unless owner |
| Capability | `can_manage_kid_logins` on co_parent for kid PIN create/reset |

**Research link:** “spouse / grandma runs the day”; tablet kid check-off; no second full admin.

---

## Wave 3 — Credible records (high school / oversight)

**Status: implemented on `main`.** Hours packaging, equal-weight course grades,
printable year report, and import merge vs replace.

### W3.1 — Hours alongside attendance

| Done | Behavior |
| --- | --- |
| Minutes on mark-done | Inline minutes field when marking a lesson done |
| Hours table | Archive shows hours by subject (+ decimal hours) with attendance |

### W3.2 — Course-level gradebook

| Done | Behavior |
| --- | --- |
| Equal-weight average | Scored tests (score ÷ out-of) average to % + letter |
| Surfaces | Subject page, archive table, year report |

### W3.3 — Transcript / report card export

| Done | Behavior |
| --- | --- |
| Year report | `GET /kids/{id}/transcript` — courses, hours, grades, attendance |
| Print | Browser print; chrome hidden via print CSS |

### W3.4 — Family import: merge vs replace

| Done | Behavior |
| --- | --- |
| Merge (default) | Add curriculum YAML + missing uploads; keep live `school.db` |
| Replace | Requires typing `REPLACE`; full wipe with prior backup |

**Research:** Tracker/Scholaric/Planet records marketing; OurSchool/Hero transcripts; WTM high-school threads.

---

## Wave 4 — Later polish (only after Waves 0–3 earn trust)

| ID | Feature | Notes |
| --- | --- | --- |
| **W4.1** | Combined day glance (adult events visible, not counted as school) | Blends life + school without polluting progress |
| **W4.2** | ICS import/export for adult or family calendar | HomeLearnAI-style; OAuth Google sync later if ever |
| **W4.3** | PWA / installable mobile shell | After kid PIN + print + Today are solid on phones |
| **W4.4** | Optional shared curriculum YAML library (opt-in) | Community share without paid marketplace |
| **W4.5** | Photo portfolio / yearbook | Panda/Moment space—only if parents ask |
| **W4.6** | Owner mailer + confirm/reset + optional OAuth | From AUTH_ROADMAP when hosting needs it |

---

## Suggested build order (summary)

```text
Wave 0  Adult-neutral → TOC/YAML/export → (auth design already done)
Wave 1  Bump policies + vacation shift → print Today/week → light onboarding
Wave 2  PIN RBAC → kid Today → secondary adult day-run  ← shipped
Wave 3  Hours → course grades → transcript → import merge/replace  ← shipped
Wave 4  Calendar glance / ICS / PWA / mailer as demand appears
```

Wave 2 auth open questions that remain are polish (teacher kid scope, adult
row linking, co_parent PIN upgrade). Wave 4 is demand-driven polish only.

## Mapping cheat sheet (research desire → feature ID)

| Parent desire (temperature) | Feature |
| --- | --- |
| Double-up vs shift-forward | W1.1 |
| Fridge/binder lists | W1.2 |
| Don’t retype every day / DIY from book TOC | W0.2–W0.4 |
| Someone else teaches a day / student list | W2.1–W2.3 |
| Grades, hours, transcripts | W3.1–W3.3 |
| Worth the money / own the data | Have + W3.4 |
| Easy setup | W1.3 |
| Marketplace packs | Skip → YAML share (W0 + W4.4) |
| Mom-default language | W0.1 |

## How to use this doc

- **Planning:** pick the next wave; break IDs into implementation todos.
- **Saying no:** if a request matches an Explicitly out of scope row, point here.
- **Refreshing research:** update [`research/competitors/`](../research/competitors/) first, then amend this roadmap—don’t grow features only from vendor feature pages.
