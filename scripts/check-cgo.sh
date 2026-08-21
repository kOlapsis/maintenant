#!/bin/sh
# Principle VI: go-sqlite3 is the only CGO dependency, and stays that way.
#
# The PostgreSQL driver was chosen pure Go precisely so this held. Nothing
# enforced it, so a dependency pulling in cgo would land unnoticed and quietly
# change what the binary can be cross-compiled for.
#
# Standard library packages that use cgo on some platforms are expected; only
# third-party ones are the question.

set -eu

allowed="github.com/mattn/go-sqlite3"

offenders=$(go list -deps -f '{{if .CgoFiles}}{{.ImportPath}}{{end}}' ./cmd/maintenant \
	| grep -E '^[a-z0-9-]+\.[a-z]+/' \
	| grep -v "^${allowed}$" || true)

if [ -n "$offenders" ]; then
	echo "CGO dependencies beyond ${allowed}:" >&2
	echo "$offenders" | sed 's/^/  /' >&2
	echo >&2
	echo "Principle VI grants exactly one CGO exception. A new one needs a" >&2
	echo "constitution amendment, not a dependency bump." >&2
	exit 1
fi

echo "CGO: ${allowed} only, as principle VI requires."
