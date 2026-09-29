#!/bin/sh
# portfind installer for Linux. No root needed.
#
#   curl -fsSL https://raw.githubusercontent.com/rpratap2111/portfind/main/install.sh | sh
#
# It downloads the release archive for your CPU, verifies its SHA-256 against
# the release's checksums.txt and installs `portfind` to ~/.local/bin.
#
# Options (environment variables):
#   PORTFIND_VERSION=v1.1.0          a specific release instead of the latest
#   PORTFIND_INSTALL_DIR=/opt/bin    somewhere other than ~/.local/bin
#   PORTFIND_DOWNLOAD_BASE=https://… a mirror hosting the release files
#
# Everything is inside main(), so a download cut off halfway runs nothing.

set -eu

err() {
	printf 'portfind installer: %s\n' "$*" >&2
	exit 1
}

have() { command -v "$1" >/dev/null 2>&1; }

download() { # url dest
	if have curl; then
		curl -fsSL -o "$2" "$1" || err "download failed: $1
Check that a release exists at https://github.com/rpratap2111/portfind/releases (and that PORTFIND_VERSION, if set, is a real tag)."
	elif have wget; then
		wget -q -O "$2" "$1" || err "download failed: $1"
	else
		err "need curl or wget"
	fi
}

sha256() {
	if have sha256sum; then
		sha256sum "$1" | cut -d' ' -f1
	elif have shasum; then
		shasum -a 256 "$1" | cut -d' ' -f1
	else
		err "need sha256sum or shasum to verify the download"
	fi
}

main() {
	repo=rpratap2111/portfind

	case "$(uname -s)" in
	Linux) os=linux ;;
	*) err "this installer is for Linux; on Windows use install.ps1 (see the README)" ;;
	esac
	case "$(uname -m)" in
	x86_64 | amd64) arch=amd64 ;;
	aarch64 | arm64) arch=arm64 ;;
	*) err "unsupported CPU '$(uname -m)'; portfind ships for x86_64 and arm64" ;;
	esac
	have tar || err "need tar"

	if [ -n "${PORTFIND_DOWNLOAD_BASE:-}" ]; then
		base=${PORTFIND_DOWNLOAD_BASE%/}
	elif [ -n "${PORTFIND_VERSION:-}" ]; then
		base="https://github.com/$repo/releases/download/$PORTFIND_VERSION"
	else
		base="https://github.com/$repo/releases/latest/download"
	fi
	dir=${PORTFIND_INSTALL_DIR:-$HOME/.local/bin}
	asset="portfind_${os}_${arch}.tar.gz"

	tmp=$(mktemp -d)
	trap 'rm -rf "$tmp"' EXIT INT TERM

	printf 'Downloading %s ...\n' "$asset"
	download "$base/$asset" "$tmp/$asset"
	download "$base/checksums.txt" "$tmp/checksums.txt"

	expected=$(awk -v f="$asset" '$2 == f || $2 == "*" f { print $1 }' "$tmp/checksums.txt")
	[ -n "$expected" ] || err "checksums.txt has no entry for $asset; refusing to install an unverified download"
	actual=$(sha256 "$tmp/$asset")
	[ "$expected" = "$actual" ] || err "checksum mismatch for $asset (expected $expected, got $actual); nothing was installed"
	printf 'Checksum verified.\n'

	tar -xzf "$tmp/$asset" -C "$tmp" portfind || err "$asset does not contain portfind"
	mkdir -p "$dir"
	# Install via a temp name and rename, so a running portfind isn't disturbed.
	cp "$tmp/portfind" "$dir/.portfind.new"
	chmod 0755 "$dir/.portfind.new"
	mv -f "$dir/.portfind.new" "$dir/portfind"

	printf '\nInstalled %s to %s\n' "$("$dir/portfind" --version)" "$dir"
	case ":$PATH:" in
	*":$dir:"*)
		printf 'Run it with:  portfind\n'
		;;
	*)
		printf '\n%s is not on your PATH. Add it, then open a new terminal:\n' "$dir"
		printf '  bash:  echo '\''export PATH="%s:$PATH"'\'' >> ~/.bashrc\n' "$dir"
		printf '  zsh:   echo '\''export PATH="%s:$PATH"'\'' >> ~/.zshrc\n' "$dir"
		printf '  fish:  fish_add_path %s\n' "$dir"
		printf 'Or run it directly:  %s/portfind\n' "$dir"
		;;
	esac
	printf 'Tip: sudo portfind shows (and can stop) other users'\'' processes too.\n'
}

main "$@"
