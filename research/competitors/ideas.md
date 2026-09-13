# Ideas backlog (from gap analysis)

Prioritized for *our* philosophy: self-hosted / family-owned data, adult + kids,
curriculum as first-class data, no marketplace lock-in.

**Canonical build plan:** research desires are folded into concrete features in
[`docs/PRODUCT_ROADMAP.md`](../../docs/PRODUCT_ROADMAP.md) (Waves 0–4). This file
stays the short brainstorm backlog; prefer the roadmap for acceptance criteria
and sequencing.

Status tags: `next` (already on the active plan) · `near` · `later` · `maybe` · `no`

## Near-term (active plan or immediate follow-ons)

| Idea | Tag | Why |
| --- | --- | --- |
| Adult-neutral copy (no Mom-default) | next | Do not assume the adult is Mom / she / her |
| TOC paste → curriculum (toc2yaml in-app) | next | Closes curriculum UX gap vs “drop in a book TOC / plan”; matches DIY curriculum scheduling from forums |
| Curriculum YAML download + in family export | next | Portable curriculum; community share without a marketplace |
| Auth roadmap (confirm email, reset, password policy, OAuth design) | next | Needed before public-ish hosted; design first |

## Strong competitive “imagine implementing” (forum temperature reinforces)

See also [`user-temperature.md`](user-temperature.md).

| Idea | Tag | Why peers sell it / how we’d do it differently |
| --- | --- | --- |
| Auto-reschedule / bump with **policy choice** (double-up vs shift-forward vs skip) | near | #1 lived desire on WTM + #1 marketed delight (Planet, Syllabird, Panda, Scholaric) |
| Printable / PDF Today & week lists per kid | near | Forums obsess over binder/fridge printables even when planning is digital |
| Kid / student check-off view (PIN) | near | Planet-era “email daily list to each student”; spouse/grandma can run a day |
| Multi-role memberships (owner / co_parent / teacher / kid) | near | Homeschool Hero + forum “someone else teaches Tue/Thu” |
| Transcript + report card export (PDF/HTML) | near | Tracker/high-school threads + SaaS/OSS parity |
| Course-level grading / GPA | near | Weighted grades repeatedly requested with attendance/hours |
| Hours / attendance reports | near | Compliance / cover-school language in older threads |
| Clear merge vs replace on family import | near | OurSchool backup modes—worth copying as *behavior*, not code |
| “Bump series earlier/later” | near | Explicit WTM ask; fits our series model |
| ICS / family calendar glance | later | Desire to blend life + school; keep schoolwork separate in data model |

## Differentiating bets (lean into what SaaS won’t)

| Idea | Tag | Why |
| --- | --- | --- |
| Curriculum YAML as interchange format | near | Import/export + TOC tool = open plans families can git/USB share |
| Adult calendar kept separate from school progress | have→polish | Already distinctive; keep sharpening |
| One-click family archive (DB + uploads + curriculum YAML) | next/near | Privacy + portability marketing |
| True local + optional hosted same binary | have→polish | Moat vs cloud-only planners |

## Later / maybe

| Idea | Tag | Notes |
| --- | --- | --- |
| Mobile PWA or lightweight native shell | later | Peers push apps; responsive web may be enough first |
| Google Calendar sync | later | Nice-to-have; self-host OAuth pain |
| Shared curriculum library (opt-in publish YAML) | maybe | Community without becoming a paid marketplace |
| Photo portfolio / yearbook | maybe | Panda/Moment space; only if parents ask |
| AI lesson suggestions | maybe | Roundups hype AI; low trust / low fit for local-first unless optional offline model |
| Family chores / shopping / meal plans | maybe | Planet/Panda life-OS creep—easy to dilute the product |
| State-specific compliance packs | maybe | HT’s wedge; high maintenance, legal footguns |
| Social network | no | Explicit skip |
| Curriculum marketplace with paid publisher packs | no | Conflicts with open YAML + self-host story (revisit only as affiliate links, not platform lock-in) |

## Suggested build order after current plan

1. Finish plan: adult-neutral copy → curriculum TOC/YAML → auth design doc  
2. Auto-reschedule / vacation shift (high perceived parity)  
3. Transcripts + stronger grading  
4. Kid check-off mode  
5. Polish hosted auth implementation when design is locked
