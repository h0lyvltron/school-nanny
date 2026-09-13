# Feature matrix (conceptual)

Legend for **School Nanny**: `have` · `partial` · `idea` · `skip`  
Legend for peers: `yes` = commonly advertised · `~` = partial / related claim · `?` = unknown · `—` = not a focus in public materials we saw

Peer columns (commercial): **HP** Homeschool Planet · **Sc** Scholaric · **Sy** Syllabird · **HT** Homeschool Tracker · **Pa** Homeschool Panda.  
Open-source shorthand: **OS** = commonly present in OurSchool / Homeschool Hero / HomeLearnAI / mblayman READMEs (see [`opensource.md`](opensource.md)).

| Capability | School Nanny | HP | Sc | Sy | HT | Pa | OS | Notes |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Multi-child week planner | have | yes | yes | yes | yes | yes | yes | Core table stakes |
| Drag / move lessons | have | yes | yes | yes | yes | yes | ~ | |
| Copy lesson to another day/child | have | ? | ? | ? | ? | ? | ? | Our Ctrl/Cmd + chip copy is a nice differentiator to keep |
| Today / daily checklist | have | yes | yes | yes | yes | yes | yes | |
| Recurring / series lessons | have | yes | yes | yes | yes | ~ | ~ | |
| Auto-reschedule after missed days | idea | yes | ~ | yes | ~ | yes | — | Highest-cited SaaS “delight”; less explicit in OSS READMEs |
| Vacation / days-off shifts schedule | idea | yes | yes | yes | ? | yes | — | Closely tied to auto-reschedule |
| Curriculum plans (TOC → lessons) | partial | yes | ~ | yes | yes | yes | yes | We have plans + YAML import; TOC-in-app + YAML export still planned |
| Publisher / marketplace lesson packs | skip* | yes | — | ~ | ~ | yes | — | *Skip as a paid marketplace; maybe later “import shared YAML” community |
| Attendance tracking | have | yes | yes | ? | yes | ~ | yes | OSS (OurSchool/Hero) treats this as core |
| Grades / gradebook | partial | yes | yes | yes | yes | yes | yes | We have tests/scores; weighted courses / GPA not there |
| Transcripts / report cards | idea | yes | yes | yes | yes | yes | yes | Strong HS / compliance sell for SaaS *and* OSS |
| State compliance packs | skip* | — | — | — | yes | — | ~ | *Maybe later as export templates, not legal advice |
| Hours tracking | idea | yes | yes | ? | yes | ? | ~ | Important in some jurisdictions |
| Student / kid logins | idea | yes | yes | yes | ? | ? | yes | OurSchool parent vs student; Hero student membership |
| Multi-role family RBAC | idea | ~ | — | — | — | — | yes | Hero: owner/parent/co-parent/tutor/student — closest to our roadmap |
| Adult / parent own calendar | have | ~ | — | — | — | — | — | Life+school calendars on Planet; our dedicated adult page is distinctive |
| Family tasks / chores / shopping | idea | yes | ~ | — | ~ | yes | — | Scope creep risk; optional later |
| File attachments on lessons | have | yes | ~ | yes | ? | yes | ~ | |
| Photo portfolio / yearbook | idea | — | — | — | ~ | yes | ~ | Panda/Moment / Offline photos |
| Mobile native apps | idea | yes | ~ | ~ | ? | yes | ~ | We are web; PWA later maybe |
| Calendar sync (Google / ICS) | idea | yes | ? | ? | ? | ? | yes | HomeLearnAI ICS import |
| Family data export / backup | have | ? | ? | ? | ~ | ? | yes | Our zip + OSS backup/restore stories |
| Self-hosted / local-first | have | — | — | — | — | — | yes | Shared philosophy with OSS peers; still rare in SaaS |
| Multi-family / hosted accounts | partial | yes | yes | yes | yes | yes | yes | Hero multi-family; we have hosted mode |
| Invite-only private deploy | have | — | — | — | — | — | ~ | Fits self-host / Coolify story |
| Spaced repetition / mastery engine | skip* | — | — | — | — | — | yes | HomeLearnAI / Homestead—different product center |
| Social / community network | skip | — | — | — | — | yes | — | Out of philosophy for us |
| Budget / expenses | skip | — | — | — | — | yes | — | Out of core scope |
| Light/dark theme | have | ? | ? | ? | ? | ? | ? | |
| Offline / LAN use | have | — | — | — | — | — | ~ | Local binary + LAN DNS; Offline app is device-local |

### Reading the matrix

Commercial peers cluster on: **planner + reschedule + records + kid login + cloud**.  
OSS peers cluster on: **self-host + attendance/grading/reports + parent/student (or richer RBAC) + backup**.  
Historical WTM forum data (2008–2020, see [`wtm-forum-sheet.md`](wtm-forum-sheet.md)) shows conversation shifting from Tracker/Skedtrack/Helper utilities toward **Homeschool Planet** (and Scholaric), i.e. all-in-one planners displacing narrower tools—useful context for where “parity” expectations came from.  
We already cover planner depth and local/self-host. Biggest gaps vs both camps: **auto-reschedule** (SaaS), **transcripts/report cards**, **kid/PIN scoped logins + RBAC** (OSS already models this), **richer grading/GPA**, and finishing **curriculum TOC/YAML UX**.
