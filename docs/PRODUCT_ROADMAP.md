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

| Behavior | Notes |
| --- | --- |
| Multi-child Today + week planner | Table stakes; keep fast |
| Drag move / Ctrl-copy / copy-to-child chips | Differentiator—keep polishing |
| Recurring series | Foundation for bump/reschedule |
| Lesson files + tests/scores | Records spine |
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
| **W0.4** | Family zip includes curriculum YAML | Archive has `curriculum/*.yaml`; restore docs say DB is authoritative | Documented restore behavior; zip contains YAML mirrors |
| **W0.5** | Auth design (done) | [`AUTH_ROADMAP.md`](AUTH_ROADMAP.md) locked enough to implement later | Roles + PIN model written; open policy questions listed |

**Research link:** DIY curriculum / TOC scheduling; setup anxiety; “share plans without marketplace.”

---

## Wave 1 — Survive the messy week (highest forum temperature)

Highest emotional parity with Planet/Scholaric/Syllabird *and* the #1 WTM ask
(“bump two ways”). Builds on series we already have.

### W1.1 — Miss-day / bump with policy picker

**Problem:** Life happens; tools that only delete or leave orphans lose trust.

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

Design is in [`AUTH_ROADMAP.md`](AUTH_ROADMAP.md). This wave **implements** the
subset that matches forum “spouse / grandma teaches Tuesday” and OSS parent/student models.

### W2.1 — Owner email + family PIN principals

**Behavior:**

- Owner: email+password (confirm/reset when mailer lands).
- Owner issues **username+PIN** for `co_parent` / `teacher` / `caregiver` / `kid`.
- Family login entry → chooser (owner email path + PIN faces).
- Owner resets any PIN; optional `can_manage_kid_logins` for kid PINs only.
- Sensitive Settings remain owner-only.

**Acceptance:** Second adult logs in with PIN and can run Today for granted kids;
kid PIN sees only own work; revoke PIN ends sessions.

### W2.2 — Kid check-off surface

**Behavior:**

- Kid session lands on **own Today** (and own week read/check-off).
- Mark done / not done; no sibling data, no Settings, no curriculum admin.
- Large tap targets (tablet).

**Acceptance:** Child completes the day without seeing another child’s lessons.

### W2.3 — Caregiver / co_parent day-run mode

**Behavior:**

- Secondary adult sees granted kids’ Today + enough lesson detail/notes/files to teach.
- Default: planning edits for co_parent; caregiver check-off / light edit (per roadmap matrix).

**Acceptance:** A written lesson note + attachment is enough for someone else to teach Tuesday without owner password.

**Research:** Planet-era student email lists + “plans detailed enough for spouse”; Homeschool Hero / OurSchool RBAC; our PIN design.

**Depends on:** Wave 0 auth design lock-in of remaining open questions (PIN length, teacher scope default, family slug vs hostname).

---

## Wave 3 — Credible records (high school / oversight)

Attendance exists; tests/scores exist. Forum + Tracker/OSS still demand packaged
**hours, course grades, transcripts**.

### W3.1 — Hours alongside attendance

**Behavior:**

- Optional duration on completed lessons and/or daily hours per kid/subject.
- Year/term report: hours by subject and total.

**Acceptance:** Parent can show “X hours of Math this year” from the app.

### W3.2 — Course-level gradebook

**Behavior:**

- Subject (or course) aggregates test scores with simple weights (equal or custom).
- Term and year letter/percent; keep existing per-test attachments.

**Acceptance:** Kid subject page shows course grade, not only individual tests.

### W3.3 — Transcript / report card export

**Behavior:**

- HTML (print) and/or PDF: student, year, courses, grades, attendance/hours summary.
- Plain, official-looking; no playful chrome.

**Acceptance:** Export opens cleanly and is usable for a portfolio or cover-school packet.

### W3.4 — Family import: merge vs replace

**Behavior:**

- On zip restore/import: explicit **Replace family data** vs **Merge** (where merge is safe) with warnings.
- Aligns with OurSchool backup semantics as *behavior*, not their code.

**Acceptance:** Docs + UI make destructive replace impossible to do by accident.

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
Wave 2  Implement PIN RBAC → kid Today → secondary adult day-run
Wave 3  Hours → course grades → transcript export → import merge/replace
Wave 4  Calendar glance / ICS / PWA / mailer as demand appears
```

Do **not** start Wave 2 implementation until remaining AUTH_ROADMAP open questions
are locked. Do **not** let Wave 4 life-OS features jump the queue ahead of bump +
print + records.

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
