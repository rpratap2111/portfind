#!/bin/sh
# portfind uninstaller for Linux.
#
#   curl -fsSL https://raw.githubusercontent.com/rpratap2111/portfind/main/uninstall.sh | sh
#
# Removes the portfind binary and the PATH lines install.sh added to
# ~/.bashrc / ~/.zshrc. Your kill history (~/.cache/portfind) is kept
# unless you set PORTFIND_PURGE_HISTORY=1. If you installed to a custom
# directory, set PORTFIND_INSTALL_DIR to it.

set -eu

main() {
	dir=${PORTFIND_INSTALL_DIR:-$HOME/.local/bin}
	data=${XDG_CACHE_HOME:-$HOME/.cache}/portfind

	if [ -f "$dir/portfind" ]; then
		rm -f "$dir/portfind"
		printf 'Removed %s/portfind\n' "$dir"
	else
		printf 'No portfind found in %s\n' "$dir"
	fi

	for rc in "$HOME/.bashrc" "$HOME/.zshrc"; do
		if [ -f "$rc" ] && grep -q '# >>> portfind >>>' "$rc"; then
			# Drop the marked block and the blank line install.sh put before it.
			tmp=$(mktemp)
			awk '/^# >>> portfind >>>$/ { skip = 1; pending = 0; next }
			     /^# <<< portfind <<<$/ { skip = 0; next }
			     skip { next }
			     $0 == "" { if (pending) print ""; pending = 1; next }
			     { if (pending) print ""; pending = 0; print }
			     END { if (pending) print "" }' "$rc" >"$tmp" && cat "$tmp" >"$rc"
			rm -f "$tmp"
			printf 'Removed the PATH lines from %s\n' "$rc"
		fi
	done

	if [ "${PORTFIND_PURGE_HISTORY:-}" = 1 ]; then
		if [ -d "$data" ]; then
			rm -rf "$data"
			printf 'Removed history in %s\n' "$data"
		fi
	elif [ -f "$data/history.db" ]; then
		printf 'Kept your history in %s (set PORTFIND_PURGE_HISTORY=1 to delete it too).\n' "$data"
	fi
	printf 'portfind uninstalled.\n'
}

main "$@"
