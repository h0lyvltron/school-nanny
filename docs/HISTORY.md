# Branching history

School Nanny keeps a per-family change tree inside that family's `school.db`.
It replaces the old lesson-only Trash workflow.

## Using it

- **Undo:** `Ctrl+Shift+Z`, or the Undo control beside History.
- **Redo:** `Ctrl+Shift+Y`, or the Redo control.
- **Choose a branch:** open **History** and select **Go here** on a retained
  change. Redo follows the branch visited most recently.

Shortcuts do not fire while typing in an input, textarea, select, or editable
element. History is shared by the family rather than by one browser tab. Every
history request includes the current node it expects; an old tab is asked to
reload instead of unexpectedly moving the shared cursor.

Undoing and then making a different edit keeps the old future as another
branch. The active path retains 128 undo steps. Branches whose fork is still in
that window remain available; older ancestry and its branches are pruned.

## Included changes

- Lessons, adult lessons, recurring series, and curriculum schedules
- Curriculum plans and items
- Attendance, assessments, and notes
- Attachments belonging to educational records
- Adult calendar events, labels, and holiday notes

One form submission is one history node, including bulk curriculum, calendar,
or schedule changes.

## Excluded changes

Accounts, sessions, PINs, passwords, profile/settings edits, timezone and week
start, backups, whole-family imports/restores, and host operations do not enter
the tree.

Deleting a kid, subject, or school year can remove parent rows required by old
planner snapshots. Those structural deletes clear planner History, and their
confirmation text says so. A replace import also starts with a clean tree.

## Attachments

Uploaded bytes are immutable. Undo and redo change attachment metadata; bytes
remain on disk while live data, a retained branch, an avatar, or a legacy
deleted-lesson record references them. Garbage collection removes unreferenced
files after history pruning.

## Legacy Trash entries

Lessons deleted before branching History was introduced retain their original
seven-day recovery window. They appear under **Legacy recently deleted** on
the History page. New lesson deletions are regular history actions.

## Storage and recovery

`history_nodes` stores the parent-linked tree and preferred redo child.
`history_changes` stores ordered before/after row snapshots. SQLite triggers
write each snapshot in the same transaction as its content row. A pending node
left by a process crash is finalized (if rows committed) or discarded when the
tenant store reopens.

Nightly backups and family exports include the history tables because they are
part of `school.db`. They remain disaster recovery, not ordinary Undo.
