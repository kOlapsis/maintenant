#!/bin/sh
# End-to-end checks against a running test stack, on either engine.
#
#   scripts/e2e-check.sh sqlite
#   scripts/e2e-check.sh postgres
#
# The point is not to re-test what the Go suites already cover, but to exercise
# the running product through its HTTP surface and assert that the behaviour is
# the same whichever engine backs it (SC-002): create monitors, drive them,
# restart the instance, and check that what should survive did.
#
# POSIX sh. Needs curl and jq.

set -eu

ENGINE="${1:-sqlite}"
BASE="${MAINTENANT_BASE_URL:-http://127.0.0.1:${E2E_HTTP_PORT:-18090}}"

COMPOSE_FILES="-f compose.test.yml"
if [ "$ENGINE" = "postgres" ]; then
	COMPOSE_FILES="-f compose.test.yml -f compose.test.postgres.yml"
elif [ "$ENGINE" != "sqlite" ]; then
	echo "usage: $0 [sqlite|postgres]" >&2
	exit 2
fi

pass=0
fail=0

ok() {
	pass=$((pass + 1))
	printf '  \033[32mok\033[0m   %s\n' "$1"
}

ko() {
	fail=$((fail + 1))
	printf '  \033[31mFAIL\033[0m %s\n' "$1"
	if [ $# -gt 1 ]; then
		printf '       %s\n' "$2"
	fi
}

# expect <description> <expected> <actual>
expect() {
	if [ "$2" = "$3" ]; then
		ok "$1"
	else
		ko "$1" "expected [$2], got [$3]"
	fi
}

api() {
	method="$1"
	path="$2"
	if [ $# -ge 3 ]; then
		curl -sS -X "$method" "$BASE$path" -H 'Content-Type: application/json' -d "$3"
	else
		curl -sS -X "$method" "$BASE$path"
	fi
}

wait_healthy() {
	i=0
	while [ $i -lt 60 ]; do
		if curl -sf "$BASE/api/v1/health" >/dev/null 2>&1; then
			return 0
		fi
		i=$((i + 1))
		sleep 1
	done
	echo "the instance never answered on $BASE" >&2
	return 1
}

echo
echo "=== end-to-end checks, engine: $ENGINE ==="
echo

wait_healthy

# --- 1. The storage the instance actually opened -----------------------------
echo "storage"
health=$(api GET /api/v1/health)
expect "health reports the $ENGINE engine" "$ENGINE" "$(echo "$health" | jq -r '.storage.engine')"
expect "the storage answers" "true" "$(echo "$health" | jq -r '.storage.connected')"
expect "no peer instance" "0" "$(echo "$health" | jq -r '.storage.peers')"

# The connection string must not surface anywhere, whatever the engine.
if echo "$health" | grep -q 'postgres://'; then
	ko "health never carries the connection string"
else
	ok "health never carries the connection string"
fi

# --- 2. Declared monitors survive a restart ----------------------------------
echo
echo "monitors"
stamp=$(date +%s)

hb=$(api POST /api/v1/heartbeats "{\"name\":\"e2e-backup-$stamp\",\"interval_seconds\":3600,\"grace_seconds\":300}")
hb_id=$(echo "$hb" | jq -r '.heartbeat.id // .id // empty')
if [ -n "$hb_id" ]; then
	ok "heartbeat created ($hb_id)"
else
	ko "heartbeat created" "$hb"
fi

ep=$(api POST /api/v1/endpoints "{\"name\":\"e2e-demo-$stamp\",\"target\":\"http://demo-web:80\",\"endpoint_type\":\"http\",\"interval\":\"30s\"}")
ep_id=$(echo "$ep" | jq -r '.endpoint.id // .id // empty')
if [ -n "$ep_id" ]; then
	ok "endpoint created ($ep_id)"
else
	ko "endpoint created" "$ep"
fi

ch=$(api POST /api/v1/channels \
	"{\"name\":\"e2e-webhook-$stamp\",\"type\":\"webhook\",\"url\":\"http://webhook-tester:8080/e2e\",\"enabled\":true}")
ch_id=$(echo "$ch" | jq -r '.channel.id // .id // empty')
if [ -n "$ch_id" ]; then
	ok "notification channel created ($ch_id)"
else
	ko "notification channel created" "$ch"
fi

# --- 3. The product runs its monitors ----------------------------------------
echo
echo "monitoring"
if [ -n "$hb_id" ]; then
	# The heartbeat's id is its ping token: the primary key is the token.
	code=$(curl -sS -o /dev/null -w '%{http_code}' "$BASE/ping/$hb_id/0")
	expect "the ping endpoint accepts the token" "200" "$code"
	sleep 2
	state=$(api GET "/api/v1/heartbeats" | jq -r "(.heartbeats // .) | map(select(.id == \"$hb_id\")) | .[0].status")
	if [ "$state" = "up" ]; then
		ok "heartbeat went up after a ping"
	else
		ko "heartbeat went up after a ping" "status is [$state]"
	fi
fi

if [ -n "$ep_id" ]; then
	api POST "/api/v1/endpoints/$ep_id/check" >/dev/null 2>&1 || true
	sleep 2
	checks=$(api GET "/api/v1/endpoints/$ep_id/checks" | jq '(.checks // .results // .) | length')
	if [ "${checks:-0}" -gt 0 ]; then
		ok "endpoint check ran and was recorded ($checks result(s))"
	else
		ko "endpoint check ran and was recorded" "no check result stored"
	fi
fi

containers=$(api GET /api/v1/containers | jq '.total // ((.groups // .containers // .) | length)')
if [ "${containers:-0}" -gt 0 ]; then
	ok "containers discovered ($containers)"
else
	ko "containers discovered" "the runtime reported nothing"
fi

# --- 4. What is written stays written ----------------------------------------
echo
echo "persistence across a restart"
# shellcheck disable=SC2086 # COMPOSE_FILES is a deliberate word list
docker compose $COMPOSE_FILES restart maintenant >/dev/null 2>&1
wait_healthy
sleep 2

after=$(api GET /api/v1/heartbeats | jq -r "(.heartbeats // .) | map(select(.id == \"$hb_id\")) | length")
expect "the heartbeat survived the restart" "1" "$after"

after_ep=$(api GET /api/v1/endpoints | jq -r "(.endpoints // .) | map(select(.id == \"$ep_id\")) | length")
expect "the endpoint survived the restart" "1" "$after_ep"

after_ch=$(api GET /api/v1/channels | jq -r "(.channels // .) | map(select(.id == \"$ch_id\")) | length")
expect "the channel survived the restart" "1" "$after_ch"

health=$(api GET /api/v1/health)
expect "still on $ENGINE after the restart" "$ENGINE" "$(echo "$health" | jq -r '.storage.engine')"
expect "the restart does not read as a second instance" "0" "$(echo "$health" | jq -r '.storage.peers')"

# --- 5. Cleanup ---------------------------------------------------------------
[ -n "$hb_id" ] && api DELETE "/api/v1/heartbeats/$hb_id" >/dev/null 2>&1
[ -n "$ep_id" ] && api DELETE "/api/v1/endpoints/$ep_id" >/dev/null 2>&1
[ -n "$ch_id" ] && api DELETE "/api/v1/channels/$ch_id" >/dev/null 2>&1

echo
echo "=== $ENGINE: $pass passed, $fail failed ==="
echo
[ "$fail" -eq 0 ]
