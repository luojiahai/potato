# Potato

A TUI for saving, fuzzy-finding, and handing off long terminal commands. Potato never executes anything — it renders a command and hands it to the parent shell or the clipboard.

## Language

**Command**:
A saved entry in the Library: a template string plus an optional description, under a unique name. Identified internally by a stable id (a UUID) so a rename keeps its State and file slot; the name is what you see, search by, and share — no two Commands may share one.
_Avoid_: snippet, alias, entry

**Library**:
The user's full set of Commands, stored as the single portable file `~/.potato/commands.json`. Copying the file is sharing the Library — there is no separate export.
_Avoid_: database, collection, config

**Placeholder**:
A `{{name}}` or `{{name=default}}` slot in a Command's template, filled in via the arg form before hand-off.
_Avoid_: variable, parameter, argument (an *argument* is the value supplied for a Placeholder)

**Hand-off**:
Delivering the rendered command to the user: Enter pre-fills the parent shell's prompt; Ctrl-Y copies to the clipboard. Potato's only output path — it never runs commands itself.
_Avoid_: execute, run (potato-side)

**State**:
The disposable per-Command cache in `~/.potato/state.json` (last-used time, last argument values), keyed by Command id so it survives renames. Safe to delete; never shared or imported.
_Avoid_: history, settings

**Import**:
Merging another Library file into your own via `potato import`. `--merge` (the default) adds every incoming Command as a new Command (fresh id), renaming on a name Collision so nothing is lost; `--override` replaces your whole Library with the imported file.
_Avoid_: sync, load, restore

**Collision**:
An incoming Command during `--merge` whose name already exists in your Library. Both are kept — the incoming one is renamed `name (N)`.
_Avoid_: conflict, duplicate
