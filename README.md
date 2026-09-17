# School Nanny

A homeschool planner and log for one family. Plan the week, mark what got done,
and keep lessons, files, tests, attendance, and notes together.

It runs on your own computer by default. You can also host it for the household,
with signup, logins, and a separate data folder per family.

## Feature list

### Day to day

- **Today:** each child's work for the day, mark done (with minutes if you want),
  and log something you never planned.
- **Also today:** household calendar items (appointments, trips) above the kid
  cards. They do not count as school hours.
- **Attendance:** present, absent, or excused on Today and on a month calendar
  with year totals.
- **Print Today / Week:** printouts meant for the fridge or a binder.
- **Messy-week tools:** Double up (pile onto the next school day), Shift / Push /
  Pull for planned work, and a vacation stretch on a curriculum plan.

### Planning the week

- **Week planner:** one column per day. Add lessons for any child. Drag to move.
  Ctrl/Cmd-drop (or **Copy to this day** on a tablet) to copy. Drop onto a child
  chip to copy work for that child. A copy from a repeating plan is always its
  own lesson, so the plan itself stays put.
- **Recurring series:** a pattern of lessons across weekdays.
- **Curriculum plans:** build or import a sequence, then schedule it onto a
  child's weekdays. Paste a book table of contents to start a plan. Download
  plans as YAML, or import YAML or CSV. From Archive, save a finished year of
  lessons back as a plan.

### Each child

- **Child page:** a card per subject with week and year progress, the next
  lesson, the last test, and a link to a printable year report.
- **Subject page:** upcoming and done lessons, subject files, tests, notes, and
  a course grade when scored tests exist.
- **Lesson page:** edit details, attach files, record a test with a score.
- **Tests:** scores (and optional graded-page uploads) per child and subject.
- **Year report:** printable summary of hours by subject, course grades,
  attendance, and test counts for a school year.
- **Archive:** a look back over a school year with hours by subject, lessons,
  tests, and notes.

### Grown-ups

- **Adult page:** pinboard cards, a week of non-school items, notes, and a month
  calendar. Adult schedule items do not count toward the children's progress.
- **Calendar export / import:** download or bring in `.ics` for the adult
  calendar.
- **Photos:** optional photos for kids and adults instead of the color dots.

### Household access (hosted mode)

- **Owner account:** email and password, with full Settings, export, and backups.
- **Household PINs:** username and short PIN for co-parents, teachers,
  caregivers, or kids. The owner issues and resets them. The login page asks
  for a family code.
- **Roles:** kids see only their own Today and Week. Caregivers can run the day
  without Settings. Co-parents and teachers can plan; only the owner can export
  or replace the family archive.

Local mode still has an optional house password for one computer.

### Your data

- **Family export / import:** a zip of the database, attachments, and curriculum
  copies. **Merge** adds plans and missing files. **Replace** asks you to type
  `REPLACE` first.
- **Backups:** automatic daily database snapshots, plus Back up now, Restore, and
  Download in Settings. Those snapshots are the database only. Use family export
  when you need the attached files too.
- **Branching History:** planner and calendar changes support persistent
  undo/redo. Undoing and editing creates another branch; see
  [`docs/HISTORY.md`](docs/HISTORY.md).
- **Installable:** the browser can install the app. A light service worker keeps
  the static shell around for revisits.

Curriculum plans are ordinary YAML (or CSV) files. There is no marketplace, social
feed, budget tracker, or chore board in this app.

## Subjects and appearance

Subjects start as Math, Language Arts, Science, Social Studies, History,
Japanese, Music & Art, and Other/Elective. Rename them, hide ones you do not
use, or add your own.

The theme control in the top bar cycles **Auto** (follow the computer),
**Light**, and **Dark**. The choice is remembered in the browser.

## Running it on a Windows computer

Build here, copy a folder there. Nothing needs to be installed on the Windows
machine.

```bash
./scripts/build-windows.sh
```

That writes `dist/windows/` with the program, a starter, and a short note. Copy
the whole folder to the PC. Double-click **Start School Nanny.bat**; a console
window opens and the browser goes to the app. Closing the console closes the app.

Records live in `%LOCALAPPDATA%\school-nanny`, not beside the program, so
replacing the program folder cannot wipe them.

- **Back up** by copying that folder somewhere safe, or use Family export in
  Settings for a zip that includes attachments.
- **Update** by replacing `school-nanny.exe`.

Windows may warn that the app is unrecognized. Choose "More info", then
"Run anyway".

To build on the Windows machine itself: `.\scripts\setup.ps1` once, then
`.\scripts\build.ps1`.

## Running it here

```bash
./scripts/setup.sh   # installs Go if needed, fetches dependencies
./scripts/run.sh     # http://127.0.0.1:8080
```

`setup.sh` uses the system Go when it is new enough, otherwise a private copy
under `.toolchain/`.

```bash
./scripts/build.sh   # produces ./school-nanny
```

### Options

| Flag | Default | What it does |
| --- | --- | --- |
| `-addr` | `127.0.0.1:8080` | Address to listen on |
| `-data` | this computer's app data folder | Folder holding the database and uploads |
| `-lan` | off | Listen on the whole home network, not just this computer |
| `-open` | off | Open a browser once the server is up |

`-lan` is how you use it from a tablet on the house Wi-Fi. Set a password in
Settings first if you do that. The console prints the addresses to open. The
Windows package also has **Start on Home Network.bat**.

On Windows, home Wi-Fi is often still marked **Public**, so port 8080 needs an
explicit firewall rule:

```powershell
.\scripts\allow-lan.ps1
```

Or from the Windows package, **Allow tablet access.bat**. Diagnose with
`.\scripts\allow-lan.ps1 -Diagnose`. Undo with `.\scripts\allow-lan.ps1 -Remove`.

## Hosted mode (optional)

For a shared household server or small VPS (Coolify-friendly), set
`SCHOOL_NANNY_MODE=hosted` and follow [`docs/HOSTED.md`](docs/HOSTED.md).
Invite-gated signup creates a family. Each family gets its own database under
the data root. See also [`docs/PUBLIC_VPS.md`](docs/PUBLIC_VPS.md) if you put it
on a public hostname.

## Where the records live

| System | Folder |
| --- | --- |
| Windows | `%LOCALAPPDATA%\school-nanny` |
| Linux | `~/.local/share/school-nanny` (or `$XDG_DATA_HOME`) |
| macOS | `~/Library/Application Support/school-nanny` |

Pass `-data <folder>` for a portable copy (for example on a USB stick). The
console prints the folder at startup.

Inside that folder, `backups\` holds database snapshots (daily and on demand).
Attachments sit in `uploads\`. Copying the whole folder is still the simplest
full backup.

## Tests

```bash
go test ./...
```

The suite drives the real HTTP handlers: planning and completing lessons,
attendance, curriculum import, family export, hosted signup and PIN logins,
hours and year reports, ICS round-trip, restore undo, and adult calendar items
staying out of kid progress.

## How it is put together

One Go binary. HTML, CSS, HTMX, and the schema are compiled in.

| Piece | Choice |
| --- | --- |
| Language | Go, standard library `net/http` and `html/template` |
| Database | SQLite through `modernc.org/sqlite` (pure Go) |
| Pages | Server-rendered HTML with HTMX for in-place updates |
| Styling | Pico CSS plus `static/app.css` |
| Schema | Numbered `.sql` files in `migrations/`, applied at startup |

Needs Go 1.25+ to build. Running needs nothing else.

### Layout

```
main.go                 startup, flags, graceful shutdown
server.go               routes, templates, sessions
hosted.go / control*.go hosted multi-family control plane
store.go                database, migrations, snapshot and restore
datadir.go / backup.go  where records live; daily snapshots
models.go               records and display helpers
ics.go                  adult calendar .ics import/export
queries_*.go            SQL
handlers_*.go           one file per area of the app
migrations/             schema changes, embedded in the binary
templates/              layout, pages, partials
static/                 HTMX, Pico, app CSS, PWA bits
docs/                   hosted deploy, auth and product notes
scripts/                setup, build, run, Windows package
```

### Data model note

A lesson is one record whether it is planned, done, or skipped. Marking planned
work done and logging something unplanned both write the same kind of row, so
the planner and the log stay aligned. Progress and hours come from those rows.
Skipped work does not count against completion.
