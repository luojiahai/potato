# docs/media/list.png — the list screen, over the fixture Library that
# scripts/screenshots.sh seeds into $POTATO_INSTALL.

Source scripts/screenshots/common.tape

# Twenty rows holding a nineteen-line frame: potato draws one row fewer than
# the terminal has, because an inline frame leaves the last one for the prompt
# to come back to.
Set Height 919

# `clear` chained onto potato's own command line rather than run ahead of it.
# potato renders inline rather than on the alternate screen, so it draws
# wherever the cursor happens to be; run alone, `clear` would be followed by a
# fresh prompt and potato would start a row underneath it.
Type "clear; $POTATO_INSTALL/bin/potato"
Enter
Sleep 2s

# right is the shutter's timing, not a navigation. potato blinks its own caret
# on a 530ms clock and restarts it solid on every keystroke, so a photograph
# taken at an arbitrary moment catches the caret lit or dark at random. right
# is the one key the list screen does not claim: it reaches the search field,
# which is empty, so it moves nothing and buys 530ms of lit caret.
Right
Sleep 300ms
Screenshot docs/media/list.png

# vhs writes a screenshot from the frame after the one it marks, so the tape
# has to outlive its own last shutter.
Sleep 1s
