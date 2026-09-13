# User temperature & desired behavior (WTM + reviews)

Goal: read **how parents talk** about planners—not vendor feature lists—and extract
recurring desires / frustrations. Sources:

1. Archived Well-Trained Mind threads linked from our mention spreadsheet
   (live WTM often 403s; Wayback snapshots used where available).
2. Public long-form reviews comparing Homeschool Planet / Tracker / Scholaric
   (secondary, 2020s language that matches the same pain points).

This is qualitative. Treat as **direction**, not a survey.

## Overall temperature

| Mood | What it sounds like |
| --- | --- |
| **Pragmatic, slightly weary** | “I’ve tried them all”; paper still wins for some; digital has to earn the subscription. |
| **Life > perfect plan** | Sick days, field trips, and “we didn’t finish” are normal; tools that fight that lose. |
| **Split brain** | Want **flexible daily chaos handling** *and* **serious records** (attendance, grades, transcripts) when high school / oversight appears. |
| **Price-sensitive** | Early Planet threads: “$65 is a lot for a planner unless it truly runs the week.” Value must be obvious. |
| **Hybrid affection** | Many still want printable day/week lists for kids/fridge/binder even if planning is digital. |
| **Setup anxiety** | Skedtrack/Tracker praise for power, complaints about hair-pulling setup; Planet praised when learning curve feels light. |

Net: parents are not asking for “more menu items.” They ask for a tool that **survives a messy week** and still produces **credible records** without becoming a second job.

## Desired behaviors (ranked by how often they show up)

### 1. Reschedule / “bump” when life happens — strongest desire

Canonical ask (WTM 2015): if we don’t finish something, I want to choose:

- **a)** pile it onto the next day (double up), **or**
- **b)** shove the whole sequence forward (add a day at the end)

Reviews still sell Planet/Scholaric/Tracker primarily on auto-reschedule after
missed days, drag-and-drop moves, and “absent → push work” helpers.

**Implication for School Nanny:** series + drag is a start; parents want an
explicit **policy picker** on miss/skip/vacation (double-up vs shift-forward vs
drop), ideally per kid/subject.

### 2. Printable kid-facing lists (digital brain, paper hands)

Threads obsess over print layouts (day/week, one child vs all children, binder
thickness). Even Planet fans highlight color/B&W weekly assignment sheets.

**Implication:** “Export/print Today or this week per child” is emotional
parity with incumbents—not a nice-to-have.

### 3. Multi-child without duplicate data entry

Want one place for the household and **per-child checklists**. Auto-fill /
repeating assignments and “don’t retype every subject every day” come up a lot.

**Implication:** we already lean this way; keep reducing re-entry (templates,
TOC→curriculum, copy-to-child).

### 4. Grades, attendance, hours, transcripts (especially older kids)

Tracker-oriented threads: weighted grades, attendance, field trips, LEA/cover
school compliance. Later reviews: report cards/transcripts “at your fingertips.”

**Implication:** lighter families may ignore this; high-school-bound families
treat it as table stakes. Our gap is real.

### 5. Daily work delivered to the student (not only the parent)

Early Planet praise: **email daily work list to each student**, device-agnostic.
Same thread: parent wants plans detailed enough that **someone else can teach
a couple days a week**.

**Implication:** kid PIN / check-off view and “hand off the day” aren’t vanity—
they match how households actually run. Secondary-adult PIN fits the “spouse /
grandma teaches Tuesday” story.

### 6. Blend school + life calendar

Desire to collapse Google Calendar / Evernote / planner into one dashboard;
family appointments beside lessons.

**Implication:** our separate adult calendar is related; optional visibility of
adult events on a family day view (without counting as schoolwork) may land well.

### 7. Mobile / tablet usability

iPad recordkeeping threads: “I love Tracker on the desktop—what works on iPad?”
Mobile is assumed for check-off and quick grade entry.

**Implication:** responsive Today + kid PIN flows matter more than a native app
at first.

### 8. Prebuilt curriculum schedules (marketplace) — quieter but sticky

Planet’s lesson-plan packs are a commercial wedge. Forum users also DIY from
book TOCs / CM lists.

**Implication:** we should prefer **TOC → curriculum YAML** (DIY import) over
building a paid marketplace; still scratch the same itch.

## Friction / anti-goals parents voice

- Steep setup that requires a “weekend to configure.”
- Paying SaaS prices for something they could do in a notebook.
- Losing paper’s glanceability (fridge list, binder).
- Tools that make looping / irregular schedules fight the data model.
- Record systems that are powerful but opaque (Skedtrack “almost pulled my hair out”).

## Temperature → School Nanny roadmap mapping

Canonical feature IDs live in [`docs/PRODUCT_ROADMAP.md`](../../docs/PRODUCT_ROADMAP.md).

| Parent desire | Our status | Feature ID | Suggested response |
| --- | --- | --- | --- |
| Bump / miss-day policies | idea | W1.1 | Explicit reschedule actions (double-up vs shift series vs skip) |
| Printable daily/weekly lists | partial | W1.2 | Polished print/PDF of Today & week per kid |
| Multi-kid planning | have | — | Keep; deepen templates / TOC import (W0.2) |
| Grades / transcripts / hours | partial → idea | W3.1–W3.3 | Course grades + transcript/report export |
| Student-facing day list | idea | W2.2 | Kid PIN → Today only |
| Someone else can teach a day | idea | W2.1–W2.3 | Caregiver/co_parent PIN + detailed lesson notes |
| School + life calendar | partial | W4.1 | Adult calendar; optional combined day glance |
| Easy onboarding | mixed | W1.3 | Avoid Tracker-style setup tax; guided year/kids/subjects |
| Curriculum packs | skip marketplace | W0.2–W0.4 | TOC paste + YAML share |
| Worth the money / self-host | have | — | Lean into local ownership as the anti-subscription answer |

## Method notes / limits

- Live WTM often blocks bots; sample is Wayback + a handful of high-signal topics
  from the spreadsheet (comparisons, Planet beta, bump-two-ways, print layouts,
  Tracker+iPad, “what’s a good planner”).
- Older threads skew CM / WTM culture; still the best longitudinal “user voice”
  attached to our mention sheet.
- Review sites amplify marketing language; used only where they echo forum pain
  (reschedule, print, records).

## Source anchors (examples)

- WTM: “Which online planner will let me bump two ways?” (2015) — double-up vs shift-forward.
- WTM: “Homeschool Planet? Anyone trying it?” (2013) — price sensitivity, student email lists, spouse can teach from plans, ease vs Tracker-class tools.
- WTM: print-layout and Tracker+iPad threads — hybrid paper + mobile records.
- Reviews (Planet / Tracker / Scholaric): auto-reschedule, attendance hours, transcripts, printables.
