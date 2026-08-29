# School Nanny

A local homeschool planner and log for one family. Plan the week, record what
actually happened, and keep the lessons, files, tests, and notes for each child
in one place.

It runs on your own computer. There is no account, no cloud, and no internet
connection needed once it is built.

## What it does

- **Today** — every child's work for the day, one tap to mark it done, plus a
  box to log something you did that was never planned.
- **Week planner** — one row per day, add lessons to any day for any child.
- **Child page** — a card per subject with progress for the week and the school
  year, the next thing coming up, and the most recent test.
- **Subject page** — upcoming and finished lessons, reusable files for that
  subject, tests, and subject notes.
- **Lesson page** — edit the details, attach files, or turn it into a test with
  a score.
- **Tests** — scores per child and subject, with the graded page attached.
- **Settings** — the kids, the subjects, the school year, and an optional
  password.

The button at the right of the top bar switches between light and dark. It
starts on **Auto**, which follows whatever the computer is set to, and clicking
it cycles to **Light** and then **Dark** if you would rather pin one. The choice
is remembered in the browser, so it is per-computer rather than per-family.

Subjects start out as Math, Language Arts, Science, Social Studies, History,
Japanese, Music & Art, and Other/Elective. You can rename them, hide the ones
you do not use, and add your own.

## Running it on her Windows computer

The usual path: build here, copy a folder there. Nothing needs to be installed
on the Windows machine.

```bash
./scripts/build-windows.sh
```

That writes `dist/windows/` containing the program, a starter, and a short note.
Copy that whole folder to her PC (USB stick, shared folder, however you like).
She double-clicks **Start School Nanny.bat**; a small black window opens and the
browser goes to the app. Closing the black window closes the app.

Her records live in `%LOCALAPPDATA%\school-nanny`, not beside the program, so
that rebuilding or replacing the program folder cannot touch them. The exact
path is on the Settings page and in the console window at startup.

- **Back up** by copying that folder somewhere safe. That is the entire backup,
  database and attachments together. The app also keeps its own daily snapshots
  of the database, which Settings can restore from.
- **Update** by replacing `school-nanny.exe`. There is nothing to preserve
  alongside it.

Windows may warn that the app is unrecognised, because it is not signed by a
company. "More info" then "Run anyway" gets past it.

If you would rather build on the Windows machine itself, run `.\scripts\setup.ps1`
once (it installs Go through winget) and then `.\scripts\build.ps1`.

## Running it here

```bash
./scripts/setup.sh   # installs Go if needed, fetches dependencies
./scripts/run.sh     # http://127.0.0.1:8080
```

`setup.sh` uses the system Go when it is new enough, and otherwise downloads a
private copy into `.toolchain/` rather than touching anything system-wide.

To build a binary for this machine instead of running from source:

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

By default the app is reachable only from the computer it runs on. `-lan` opens
it to other devices on the house network, which is how you would use it from a
tablet. Set a password in Settings first if you do that.

## Where the records live, and backups

The database and the attachments sit in one folder per computer:

| System | Folder |
| --- | --- |
| Windows | `%LOCALAPPDATA%\school-nanny` |
| Linux | `~/.local/share/school-nanny` (or `$XDG_DATA_HOME`) |
| macOS | `~/Library/Application Support/school-nanny` |

It is deliberately nowhere near the source tree or the build output. An earlier
version defaulted to a `data` folder beside whatever started the app, which
meant `run.ps1` and the Windows launcher quietly used two different databases,
and a rebuild that cleared `dist\windows` took the records with it. Starting the
app from anywhere now finds the same records, and no build touches them.

The first run after that change copies an older `data` folder into the new home
if it finds one, leaving the original where it was and saying so in the console.
Pass `-data <folder>` to override all of this, for a portable copy on a USB
stick; an explicit folder is taken at face value and nothing is copied into it.

Inside that folder, `backups\` holds snapshots of the database:

- One is taken automatically the first time the app opens each day.
- **Back up now** in Settings takes one on demand.
- **Restore** puts the records back to a snapshot, after first snapshotting what
  it is about to replace, so restoring the wrong one is itself undoable.
- **Download** saves a snapshot through the browser, which is how you get one
  off this computer.

A snapshot is the database only. Attachments are ordinary files in `uploads\`,
so copying the whole folder remains the real backup, and the one worth keeping
somewhere other than this machine.

## Tests

```bash
go test ./...
```

The suite drives the real HTTP handlers: planning a lesson and completing it,
logging work after the fact, overdue work reaching the home page, file upload
and download, scores, notes, and the password lock.

It also covers the things that would be expensive to get wrong: that a restore
puts the right records back and can itself be undone, that the app keeps serving
and writing after the database is swapped underneath it, that a file which is
not a backup is refused, and that an older data folder is adopted exactly once
and left in place.

## How it is put together

One Go binary. The HTML, the stylesheet, HTMX, and the database schema are all
compiled into it, so the program plus its `data` folder is the whole app.

| Piece | Choice |
| --- | --- |
| Language | Go, standard library `net/http` and `html/template` |
| Database | SQLite through `modernc.org/sqlite` (pure Go, no C compiler) |
| Pages | Server-rendered HTML with HTMX for in-place updates |
| Styling | Pico CSS plus `static/app.css` |
| Schema changes | Numbered `.sql` files in `migrations/`, applied at startup |

Building needs Go 1.25 or newer. Running needs nothing at all.

### Layout

```
main.go                 startup, flags, graceful shutdown
server.go               routes, templates, sessions, password hashing
store.go                database connection, migrations, snapshot and restore
datadir.go              where the records live, and adopting an older folder
backup.go               daily snapshots, restoring, pruning
models.go               the records and how they are displayed
queries_*.go            SQL for people, lessons, and records
handlers_*.go           one file per area of the app
migrations/             numbered schema changes, embedded in the binary
templates/              layout, one file per page, shared partials
static/                 HTMX, Pico CSS, the app stylesheet, the theme switch
scripts/                setup, build, run, and the Windows package
```

### A note on the two themes

`app.css` names every colour once as a token and then gives that token a light
value and a dark one. Nothing below the palette blocks hardcodes a colour, so
adding a screen costs no theme work. The head of every page resolves the choice
to `data-theme="light"` or `data-theme="dark"` before the stylesheet paints,
which keeps both palettes as plain attribute selectors and means there is no
`prefers-color-scheme` block to keep in step with them.

### A note on the data model

A lesson is one record whether it is planned, done, or skipped. Marking a
planned lesson done and logging something you never planned both write the same
row, which is why the planner and the logbook never disagree.

Progress is counted from those rows rather than tracked separately, so there is
nothing to keep in sync. Completion ignores skipped lessons: deciding not to do
something should not look like falling behind.
