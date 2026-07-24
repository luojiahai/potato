#!/usr/bin/env bash
# potato installer (spec §6.2). Everything lands under ~/.potato (POTATO_INSTALL
# overrides); no sudo, no PATH edit. Advertised install:
#
#   curl -fsSL https://raw.githubusercontent.com/luojiahai/potato/main/install.sh | bash \
#     && source ~/.potato/init.sh
#
# An optional version argument pins a release: `... | bash -s -- 1.0.0`
set -euo pipefail

REPO="luojiahai/potato"
INSTALL_DIR="${POTATO_INSTALL:-$HOME/.potato}"
BIN_DIR="$INSTALL_DIR/bin"
VERSION="${1:-latest}"

say() { printf '%s\n' "$*"; }
fail() { printf 'potato install: %s\n' "$*" >&2; exit 1; }

detect_target() {
  local os arch
  os="$(uname -s)"
  arch="$(uname -m)"
  case "$os" in
    Darwin) os=darwin ;;
    Linux) os=linux ;;
    *) fail "unsupported OS: $os (potato supports macOS and Linux)" ;;
  esac
  case "$arch" in
    x86_64 | amd64) arch=x64 ;;
    arm64 | aarch64) arch=arm64 ;;
    *) fail "unsupported architecture: $arch" ;;
  esac
  # a shell under Rosetta 2 reports x86_64 on Apple silicon — install the native binary
  if [ "$os" = darwin ] && [ "$arch" = x64 ] \
    && [ "$(sysctl -n sysctl.proc_translated 2>/dev/null || echo 0)" = 1 ]; then
    arch=arm64
  fi
  printf '%s-%s' "$os" "$arch"
}

sha256_check() {
  # $1 = file, $2 = expected hash
  local actual
  if command -v sha256sum >/dev/null 2>&1; then
    actual="$(sha256sum "$1" | awk '{print $1}')"
  else
    actual="$(shasum -a 256 "$1" | awk '{print $1}')"
  fi
  [ "$actual" = "$2" ] || fail "sha256 mismatch for $(basename "$1"): expected $2, got $actual"
}

main() {
  local target asset base tmp expected rc_line rc_file shell_name existed_before
  target="$(detect_target)"
  asset="potato-${target}.tar.gz"

  if [ "$VERSION" = latest ]; then
    base="https://github.com/$REPO/releases/latest/download"
  else
    base="https://github.com/$REPO/releases/download/v${VERSION#v}"
  fi

  tmp="$(mktemp -d "${TMPDIR:-/tmp}/potato-install.XXXXXX")"
  trap 'rm -rf "$tmp"' EXIT

  say "downloading $asset…"
  curl -fsSL -o "$tmp/$asset" "$base/$asset" || fail "download failed: $base/$asset"
  curl -fsSL -o "$tmp/SHA256SUMS" "$base/SHA256SUMS" || fail "download failed: $base/SHA256SUMS"

  expected="$(awk -v f="$asset" '$2 == f || $2 == "*" f {print $1}' "$tmp/SHA256SUMS")"
  [ -n "$expected" ] || fail "SHA256SUMS has no entry for $asset"
  sha256_check "$tmp/$asset" "$expected"

  mkdir -p "$BIN_DIR"
  tar -xzf "$tmp/$asset" -C "$tmp"
  chmod 0755 "$tmp/potato"
  mv -f "$tmp/potato" "$BIN_DIR/potato" # always overwritten on re-run

  # static shell glue, regenerated from the binary on every install/update
  "$BIN_DIR/potato" init zsh > "$INSTALL_DIR/init.zsh"
  "$BIN_DIR/potato" init bash > "$INSTALL_DIR/init.bash"
  "$BIN_DIR/potato" init sh > "$INSTALL_DIR/init.sh"

  # one rc line for the login shell, appended only once (grep-guarded)
  shell_name="$(basename "${SHELL:-}")"
  existed_before=no
  case "$shell_name" in
    zsh) rc_file="$HOME/.zshrc"; rc_line="source $INSTALL_DIR/init.zsh" ;;
    bash) rc_file="$HOME/.bashrc"; rc_line="source $INSTALL_DIR/init.bash" ;;
    *) rc_file=""; rc_line="source $INSTALL_DIR/init.sh" ;;
  esac
  if [ -n "$rc_file" ]; then
    if grep -qs 'potato/init\.' "$rc_file" 2>/dev/null; then
      existed_before=yes
    else
      touch "$rc_file"
      printf '\n%s\n' "$rc_line" >> "$rc_file"
    fi
  fi

  local installed_version
  installed_version="$("$BIN_DIR/potato" --version)"
  say ""
  say "🥔 potato $installed_version installed ($target)"
  if [ "$existed_before" = yes ]; then
    # re-run: just the version change, no activation instructions
    return
  fi
  say "   binary: $BIN_DIR/potato"
  say "   shell glue: $INSTALL_DIR/init.{zsh,bash,sh}"
  if [ -n "$rc_file" ]; then
    say "   added to $rc_file: $rc_line"
    if [ "$shell_name" = bash ] && [ "$(uname -s)" = Darwin ]; then
      say "   note: on macOS, make sure ~/.bash_profile sources ~/.bashrc"
    fi
  else
    say "   unsupported login shell ($shell_name) — add this line to your shell's rc yourself:"
    say "     $rc_line"
  fi
  say ""
  say "If potato isn't available in this shell, run: source $INSTALL_DIR/init.sh"
}

main
