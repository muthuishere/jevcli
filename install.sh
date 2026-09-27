#!/bin/sh
# jevcli installer (macOS / Linux): downloads the release binary, then `jevcli install` puts the agent skill in
# ~/.claude/skills, ~/.agents/skills (and ~/.codex/skills if present) and adds the Claude Code hook template (disabled).
#   curl -fsSL https://muthuishere.github.io/jevcli/install.sh | sh
#   JEVCLI_VERSION=v0.1.0 JEVCLI_BIN=~/.local/bin sh install.sh     # pin a version / choose the dir
#   JEVCLI_NO_HOOK=1 ...                                            # skills only
set -eu
REPO=muthuishere/jevcli
BIN=${JEVCLI_BIN:-$HOME/.local/bin}
VER=${JEVCLI_VERSION:-latest}
os=$(uname -s | tr '[:upper:]' '[:lower:]')
case $(uname -m) in x86_64|amd64) arch=amd64 ;; arm64|aarch64) arch=arm64 ;; *) echo "jevcli: unsupported arch $(uname -m)" >&2; exit 1 ;; esac
case $os in darwin|linux) ;; *) echo "jevcli: unsupported OS $os (Windows: install.cmd)" >&2; exit 1 ;; esac
asset=jevcli_${os}_${arch}
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
if [ "$VER" = latest ]; then url=https://github.com/$REPO/releases/latest/download/$asset
else url=https://github.com/$REPO/releases/download/$VER/$asset; fi
echo "jevcli: downloading $asset ($VER)"
if ! curl -fsSL "$url" -o "$tmp/jevcli"; then
  # a private repo needs an authenticated download
  command -v gh >/dev/null || { echo "jevcli: download failed: $url" >&2; exit 1; }
  if [ "$VER" = latest ]; then gh release download -R $REPO -p "$asset" -D "$tmp"; else gh release download "$VER" -R $REPO -p "$asset" -D "$tmp"; fi
  mv "$tmp/$asset" "$tmp/jevcli"
fi
chmod +x "$tmp/jevcli"
mkdir -p "$BIN"; mv "$tmp/jevcli" "$BIN/jevcli"
echo "jevcli: installed $("$BIN/jevcli" version) to $BIN/jevcli"
if [ -n "${JEVCLI_NO_HOOK:-}" ]; then "$BIN/jevcli" install --no-hook; else "$BIN/jevcli" install; fi
case ":$PATH:" in *":$BIN:"*) ;; *) echo "jevcli: add $BIN to your PATH" ;; esac
echo "jevcli: next: jevcli profile add jev URL --model M --header \"Authorization: Bearer \$YOUR_KEY_VAR\""
