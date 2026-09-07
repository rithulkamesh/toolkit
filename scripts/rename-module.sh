#!/usr/bin/env sh
# Rename the Go module path across the whole repo in one shot.
#
# Usage:
#   scripts/rename-module.sh github.com/you/toolkit
#
# Uses GNU sed (`sed -i`). On macOS, install gnu-sed or change `sed -i` to
# `sed -i ''` below.
set -eu

if [ $# -ne 1 ]; then
	echo "usage: $0 <new-module-path>" >&2
	exit 1
fi

OLD="$(awk '/^module /{print $2; exit}' go.mod)"
NEW="$1"

if [ "$OLD" = "$NEW" ]; then
	echo "module path is already $NEW — nothing to do"
	exit 0
fi

echo "renaming module: $OLD  ->  $NEW"
# Every tracked file that mentions the old path (imports, go.mod, docs).
grep -rl --exclude-dir=.git -F "$OLD" . | while IFS= read -r f; do
	sed -i "s|$OLD|$NEW|g" "$f"
	echo "  updated $f"
done

if command -v go >/dev/null 2>&1; then
	go mod tidy
	echo "ran: go mod tidy"
fi

echo "done. verify with: go build ./... && go test ./..."
