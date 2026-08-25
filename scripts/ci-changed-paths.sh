#!/bin/sh
# Classifies the paths a run touches, so CI can skip the jobs whose verdict
# cannot have changed. Writes `backend` and `frontend` to $GITHUB_OUTPUT.
#
# Fail-open by design: no diff range, a git diff that fails, an empty diff, or
# a path matching no known category turns everything on. A pipeline that runs
# too much is an inconvenience; a check skipped by mistake is a bug.
#
# The range comes from the environment, never interpolated into a workflow
# run: block — see the hardening note at the top of build-docker.yml.
#   BASE_SHA  pull_request: the base commit. push: the commit before.
#             Empty on every other event, which is what makes a release,
#             a schedule or a manual dispatch run the whole pipeline.
#   HEAD_SHA  the commit under test, HEAD if unset.
set -eu

backend=false
frontend=false
reason=""

everything() {
	backend=true
	frontend=true
}

base="${BASE_SHA:-}"
head="${HEAD_SHA:-HEAD}"
zero="0000000000000000000000000000000000000000"

files=""
if [ -z "$base" ] || [ "$base" = "$zero" ]; then
	reason="no diff range for this event"
	everything
elif ! files=$(git diff --name-only "$base" "$head" 2>/dev/null); then
	reason="git diff ${base}..${head} failed"
	everything
elif [ -z "$files" ]; then
	reason="empty diff for ${base}..${head}"
	everything
fi

if [ -z "$reason" ]; then
	list=$(mktemp)
	printf '%s\n' "$files" >"$list"

	# A path can only fall in one bucket, so order matters: the inert list
	# wins over everything, then the frontend, then the backend. What no
	# pattern claims is unknown, and unknown means run it all.
	while read -r f; do
		[ -n "$f" ] || continue
		case "$f" in
		# Nothing in CI reads these. The docs have their own workflow.
		docs/* | mkdocs.yml | deploy/* | *.md | LICENSE | .env.example | .gitignore) ;;
		frontend/*)
			frontend=true
			;;
		# Grouped because they decide the same verdicts: the Makefile, the
		# test compose files and the e2e script drive `make e2e-both`, which
		# sits at the end of the sast → lint → test → e2e chain.
		*.go | go.mod | go.sum | cmd/* | internal/* | proto/* | scripts/* | \
			Makefile | Dockerfile | docker-entrypoint.sh | .dockerignore | \
			.trivyignore | .golangci.yml | codeql-config.yml | compose*.yml)
			backend=true
			;;
		*)
			reason="unclassified path: ${f}"
			everything
			break
			;;
		esac
	done <"$list"

	rm -f "$list"
fi

if [ -n "${GITHUB_OUTPUT:-}" ]; then
	{
		echo "backend=${backend}"
		echo "frontend=${frontend}"
	} >>"$GITHUB_OUTPUT"
fi

if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
	{
		echo "| Category | Runs |"
		echo "|---|---|"
		echo "| backend | \`${backend}\` |"
		echo "| frontend | \`${frontend}\` |"
		[ -z "$reason" ] || echo ""
		[ -z "$reason" ] || echo "Everything runs: ${reason}."
	} >>"$GITHUB_STEP_SUMMARY"
fi

echo "backend=${backend} frontend=${frontend}"
[ -z "$reason" ] || echo "everything runs: ${reason}"
