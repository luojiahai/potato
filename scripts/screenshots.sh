#!/usr/bin/env bash
# Retake the README's screenshots (docs/media/list.png, docs/media/arguments.png).
#
# scripts/screenshots renders the screens to ANSI over a fixture library and a
# fixed clock; freeze turns each into a PNG. The flags below are the house
# style, and are what keeps the two images looking like one set: potato's own
# rule colour around a warm-dark window, and Menlo, which is on every mac and
# carries the box-drawing glyphs the frame is built from.
#
# Needs freeze: go install github.com/charmbracelet/freeze@latest
set -euo pipefail
cd "$(dirname "$0")/.."

# What the header rule says the running potato is. The latest tag by default —
# the version someone installing today would see.
version="${1:-$(git describe --tags --abbrev=0 2>/dev/null || echo dev)}"
version="${version#v}"

if ! command -v freeze >/dev/null 2>&1; then
  echo "screenshots: freeze not found — go install github.com/charmbracelet/freeze@latest" >&2
  exit 1
fi

background="#14100a" # the terminal under potato, a shade off its own ink
rule="#5c4a2e"       # ruleColor: the window border is the frame's own hairline

frames="$(mktemp -d)"
trap 'rm -rf "$frames"' EXIT

go run ./scripts/screenshots -out "$frames" -version "$version" -background "$background"

for screen in list arguments; do
  # Padding is short at the bottom because freeze reserves a line's descent
  # there that nothing is drawn in; trimming it leaves the frame centred.
  freeze "$frames/$screen.ansi" -o "docs/media/$screen.png" \
    --font.family Menlo --font.size 8 --line-height 1.4 \
    --background "$background" --margin 10 --padding 14,14,2,14 \
    --border.radius 6 --border.width 1 --border.color "$rule"
done
