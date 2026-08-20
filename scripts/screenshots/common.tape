# The settings both screenshot tapes share. Sourced rather than copied: the
# two screens differ only in how tall they are and what is typed at them, and
# a setting that drifted between the tapes would show up in the README as two
# images that no longer look like one pair.
#
# Height is deliberately not here. It is the one setting the two screens
# disagree on, and it has to be set by the caller before its first command:
# vhs applies the settings block once, up front, and a Set that arrives after
# any command is ignored without a word.
#
# A sourced path resolves against the working directory rather than the
# sourcing tape's own, which is why scripts/screenshots.sh runs vhs from the
# repo root.

Set Shell bash
Set FontFamily "Menlo"
Set FontSize 32
Set LineHeight 1.0
Set Padding 56
Set Margin 0

# The ground potato is drawn on, and its own textColor (both from
# internal/tui/ui.go). Nothing else needs naming here: every other colour in
# the frame is a truecolor escape potato writes itself, so the theme has only
# to supply what it draws on and the ink it falls back to.
Set Theme { "background": "#14100a", "foreground": "#e4dccf" }

# Set Theme carries that background into the terminal but not into MarginFill,
# which is what vhs paints the whole canvas with. Left alone MarginFill keeps
# vhs's own default, and that default reaches the image even at Margin 0 — a
# two-pixel edge down the right and along the bottom, in a grey lighter than
# anything potato draws.
Set MarginFill "#14100a"

Set BorderRadius 0
Set CursorBlink false

# Pixels, not cells: vhs sizes the viewport to Width - 2*Padding and lets
# xterm.js fit a grid into it. 1749 lands exactly 80 columns. The band that
# does runs 1738..1767, so this sits in the middle of it rather than on an
# edge, where a font metric that moved by a hair would cost or gain a column.
Set Width 1749
