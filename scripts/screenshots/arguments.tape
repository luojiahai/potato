# docs/media/arguments.png — the arg screen, reached the way a user reaches it.
#
# A second tape rather than a second Screenshot in the first one: vhs sizes the
# terminal once, before any command runs, so one tape is one size. The arg
# screen wants ten rows, and taking it at the list's twenty would leave eleven
# empty ones between "Will run" and the footer.

Source scripts/screenshots/common.tape

# Ten rows holding a nine-line frame — see list.tape on the row potato leaves.
Set Height 507

Type "clear; $POTATO_INSTALL/bin/potato"
Enter
Sleep 2s

# Type until "tail app logs" is the selection, then Enter: the query narrows
# the list to it, so this is the route the README describes rather than a
# shortcut into the screen.
Type "app logs"
Sleep 500ms
Enter

# Enter doubles as the caret's timing here — it restarts potato's blink solid
# on the way into this screen, so the shutter 300ms later catches the caret
# lit. list.tape has to spend a keystroke to say the same thing.
Sleep 300ms
Screenshot docs/media/arguments.png
Sleep 1s
