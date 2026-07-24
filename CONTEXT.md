# Potato

A TUI for saving, fuzzy-finding, and handing off long terminal commands. Potato never executes anything — it renders a command and hands it to the parent shell or the clipboard.

## Language

**Command**:
A saved, named entry in the Library: a template string plus an optional description. Identified everywhere by its name.
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
The disposable per-Command cache in `~/.potato/state.json` (last-used time, last argument values). Safe to delete; never shared or imported.
_Avoid_: history, settings

**Import**:
Merging another Library file into your own via `potato import`. Ours-wins: it only adds Commands whose names you don't already have, unless overwriting is explicitly requested.
_Avoid_: sync, load, restore

**Collision**:
An incoming Command during Import whose name already exists in your Library. Differing Collisions are skipped and reported; identical ones are no-ops.
_Avoid_: conflict, duplicate
