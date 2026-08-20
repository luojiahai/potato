#!/usr/bin/env bash
# Retake the README's screenshots (docs/media/list.png, docs/media/arguments.png).
#
# Both come from the real binary running in a real terminal: vhs drives potato
# under ttyd in headless Chrome and screenshots it. That is the whole reason for
# the pipeline. A rasteriser has to reimplement a terminal, and the one this
# script replaced got two things wrong that the README showed — it drew the
# potato as a flat silhouette, having no way to read a colour emoji, and it drew
# the search caret half a cell too wide, padding every background run. A
# terminal gets both right by being one.
#
# The library is a fixture, not whatever is in ~/.potato: a screenshot has to be
# reproducible, and the README should show potato holding a plausible day's work
# rather than the author's. POTATO_INSTALL puts it somewhere disposable, which
# is also where the binary is built, so the screens are always HEAD's.
#
# Needs vhs: brew install vhs
set -euo pipefail
cd "$(dirname "$0")/.."

# What the header rule says the running potato is. The latest tag by default —
# the version someone installing today would see.
version="${1:-$(git describe --tags --abbrev=0 2>/dev/null || echo dev)}"
version="${version#v}"

if ! command -v vhs >/dev/null 2>&1; then
  echo "screenshots: vhs not found — brew install vhs" >&2
  exit 1
fi

home="$(mktemp -d)"
trap 'rm -rf "$home"' EXIT

# The last-used times are written relative to now, so "2h ago" reads the same
# whenever the screenshots are retaken. timeAgo truncates, so a capture that
# starts a minute later still rounds down to the same label.
ago() { date -u -v-"$1" +%Y-%m-%dT%H:%M:%SZ; }

cp scripts/screenshots/commands.json "$home/commands.json"
sed -e "s/@2H@/$(ago 2H)/" \
    -e "s/@20H@/$(ago 20H)/" \
    -e "s/@48H@/$(ago 48H)/" \
    scripts/screenshots/state.json.tmpl > "$home/state.json"

go build -ldflags "-X github.com/luojiahai/potato/internal/version.Version=${version}" \
  -o "$home/bin/potato" ./cmd/potato

# vhs hands its own environment to the shell it opens, so the tapes read
# $POTATO_INSTALL rather than carrying a path that would have to be templated in.
export POTATO_INSTALL="$home"

for screen in list arguments; do
  echo "screenshots: ${screen}…"
  out="docs/media/$screen.png"
  # vhs answers a runtime failure with a message and an exit status of 0 — a
  # tape that opens no terminal, or whose last shutter falls off the end of the
  # recording, leaves the previous PNG sitting there and says nothing. Taking
  # the file away first is what turns that into a stop rather than a stale
  # image committed by mistake.
  rm -f "$out"
  vhs "scripts/screenshots/$screen.tape"
  if [ ! -f "$out" ]; then
    echo "screenshots: $screen.tape recorded nothing — $out was not written" >&2
    exit 1
  fi
done
