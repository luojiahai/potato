# Potato

A TUI for saving, fuzzy-finding, and handing off long terminal commands. Potato never executes anything — it renders a command and hands it to the parent shell or the clipboard.

## Language

**Command**:
A saved entry in the Library: a template string plus an optional description, under a unique name. Identified internally by a stable id (a UUID) so a rename keeps its State and file slot; the name is what you see, search by, and share — no two Commands may share one.
_Avoid_: snippet, alias, entry

**Library**:
The user's full set of Commands, stored as the single portable file `~/.potato/commands.json`. Copying the file is sharing the Library — there is no separate export.
_Avoid_: database, collection, config

**Draft**:
A Command's content without its identity — a name, an optional description, and a template string. What the add/edit screen's Form holds, and what the Library accepts when adding or updating a Command. A Draft becomes a Command when the Library gives it an id.
_Avoid_: input, params (a Draft is what a Form's values become)

**Field**:
One editable value on screen — its text, its caret, and how it draws itself into the width it is given. The search field, the three on the add/edit screen, and one per Placeholder on the arg screen. A Field does not decide how wide it is: it is told, by a Layout on the list and arg screens and by the section it sits in on the add/edit screen. Potato paints every Field's caret itself; the library underneath keeps the value, the cursor and the readline keys.
_Avoid_: input, textbox

**Form**:
The ordered Fields a screen holds, with the one focus ring that moves the keyboard between them. Tab is the Form's key, not a screen's. The add/edit screen and the arg screen each hold one; the list screen does not — its search field alone holds the keyboard, and every verb is a chord the field does not claim.
_Avoid_: fieldset, dialog

**Layout**:
The cells one line of the frame is laid out from, and the candidate arrangements that fit them to a width. A list row, an arg row and the search row each go through one, and the Layout owns all four of the decisions they share: how wide each column is, measured across the whole *block* — the lines laid out together, which is why a Layout is handed all of them at once and never one alone — so the columns line up down every one of them; what the line gives up as the terminal narrows; the padding between what is left; and the selection bar running unbroken through every run and every pad of it.
_Avoid_: row (a row is one line of the frame, laid out or not), grid, table

**Placeholder**:
A `{{name}}` or `{{name=default}}` slot in a Command's template, filled in via the arg form before hand-off. Writing one without a default makes it required, and the arg form refuses to run or copy while it is empty; `{{name=}}` is how a Placeholder that may be left empty is written.
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
