#!/usr/bin/env bats

SCRIPT="$BATS_TEST_DIRNAME/../../ansible/roles/cluster_alert/files/maintenant-cluster-alert"

setup() {
	fake="$BATS_TEST_TMPDIR/bin"
	mkdir -p "$fake"
	export LOG="$BATS_TEST_TMPDIR/journal" POSTS="$BATS_TEST_TMPDIR/posts" CURL_ARGS="$BATS_TEST_TMPDIR/curl-args"
	export OM_STATUS="$BATS_TEST_TMPDIR/om-status.json"
	cat >"$fake/logger" <<'EOF'
#!/bin/sh
printf '%s\n' "$*" >>"$LOG"
EOF
	cat >"$fake/curl" <<'EOF'
#!/bin/sh
printf '%s\n' "$*" >>"$CURL_ARGS"
[ -z "${CURL_FAILS:-}" ] || exit 7
cat >>"$POSTS"
EOF
	cat >"$fake/om" <<'EOF'
#!/bin/sh
[ -z "${OM_DELAY:-}" ] || sleep "$OM_DELAY"
cat "$OM_STATUS"
EOF
	chmod +x "$fake"/*
	export PATH="$fake:$PATH"
	export CLUSTER_ALERT_CONF="$BATS_TEST_TMPDIR/conf"
	state="$BATS_TEST_TMPDIR/state"
	printf 'state_dir=%s\nwebhook=http://receiver.test/\ncluster=lab\nsettle_s=0\n' "$state" >"$CLUSTER_ALERT_CONF"
	host=$(uname -n | tr '[:upper:]' '[:lower:]')
	started="2026-01-01T00:00:00.123456789+01:00"
	arbitrator=up
	hb1=true
	hb2=true
	hb2tx=true
}

om_reports() {
	jq -n --arg h "$host" --arg started "$started" --arg a "$arbitrator" \
		--argjson hb1 "$hb1" --argjson hb2 "$hb2" --argjson hb2tx "$hb2tx" '
		{cluster: {node: {($h): {
			status: {arbitrators: {"0": {url: "http://10.10.9.13:1216/arbitrator", status: $a, weight: 1}}},
			daemon: {started_at: $started, heartbeat: {streams: [
				{id: "hb#1.rx", type: "unicast", peers: {"site-b": {is_beating: $hb1}}},
				{id: "hb#1.tx", type: "unicast", peers: {"site-b": {is_beating: $hb1}}},
				{id: "hb#2.rx", type: "relay", peers: {"site-b": {is_beating: $hb2}}},
				{id: "hb#2.tx", type: "relay", peers: {"site-b": {is_beating: $hb2tx}}}
			]}}
		}}}}' >"$OM_STATUS"
}

check() {
	om_reports
	sh "$SCRIPT" --check
}

posts() {
	[ ! -e "$POSTS" ] || jq -c 'del(.at)' "$POSTS"
}

events() {
	[ ! -e "$POSTS" ] || jq -r '.event + (if .hb then " " + .hb else "" end)' "$POSTS" | paste -sd, -
}

arbitrator_error='{"kind":"ArbitratorError","id":1,"at":"0001-01-01T00:00:00Z","data":{"labels":{},"node":"site-a","name":"0","error":"Get \"http://10.10.9.13:1216/arbitrator\": context deadline exceeded"}}'

@test "a healthy cluster is no alert" {
	check
	check
	[ ! -e "$POSTS" ]
	[ ! -e "$LOG" ]
}

@test "a lost arbitrator alerts once, with the error of the event that triggered the check" {
	arbitrator=down
	om_reports
	EVENT=$arbitrator_error sh "$SCRIPT" --check
	check
	[ "$(posts)" = "{\"event\":\"arbitrator_down\",\"cluster\":\"lab\",\"node\":\"$host\",\"arbitrator\":\"0\",\"error\":\"Get \\\"http://10.10.9.13:1216/arbitrator\\\": context deadline exceeded\",\"text\":\"$host: arbitrator 0 is unreachable: Get \\\"http://10.10.9.13:1216/arbitrator\\\": context deadline exceeded\"}" ]
	[ "$(cat "$LOG")" = "-t maintenant-cluster -p user.warning -- event=arbitrator_down cluster=lab node=$host arbitrator=0 error=\"Get \\\"http://10.10.9.13:1216/arbitrator\\\": context deadline exceeded\"" ]
}

@test "a lost arbitrator the timer finds carries an empty error" {
	arbitrator=down
	check
	[ "$(posts)" = "{\"event\":\"arbitrator_down\",\"cluster\":\"lab\",\"node\":\"$host\",\"arbitrator\":\"0\",\"error\":\"\",\"text\":\"$host: arbitrator 0 is unreachable\"}" ]
}

@test "the arbitrator back alerts once" {
	arbitrator=down
	check
	arbitrator=up
	check
	check
	[ "$(events)" = "arbitrator_down,arbitrator_up" ]
	[ "$(posts | tail -n 1)" = "{\"event\":\"arbitrator_up\",\"cluster\":\"lab\",\"node\":\"$host\",\"arbitrator\":\"0\",\"text\":\"$host: arbitrator 0 answers again\"}" ]
	[ "$(tail -n 1 "$LOG")" = "-t maintenant-cluster -p user.notice -- event=arbitrator_up cluster=lab node=$host arbitrator=0" ]
	[ ! -e "$state/arbitrator-0" ]
}

@test "a receive stream alerts when it goes stale and when it beats again, a send stream never" {
	hb2=false
	hb2tx=false
	check
	check
	hb2=true
	hb2tx=true
	check
	[ "$(posts)" = "{\"event\":\"heartbeat_stale\",\"cluster\":\"lab\",\"node\":\"$host\",\"peer\":\"site-b\",\"hb\":\"hb#2.rx\",\"text\":\"$host: heartbeat hb#2.rx with site-b is stale\"}
{\"event\":\"heartbeat_alive\",\"cluster\":\"lab\",\"node\":\"$host\",\"peer\":\"site-b\",\"hb\":\"hb#2.rx\",\"text\":\"$host: heartbeat hb#2.rx with site-b beats again\"}" ]
}

@test "a peer with no receive stream beating is stale, and alive again once one beats" {
	hb1=false
	hb2=false
	check
	[ "$(events)" = "heartbeat_stale hb#1.rx,heartbeat_stale hb#2.rx,node_stale" ]
	hb2=true
	check
	[ "$(events)" = "heartbeat_stale hb#1.rx,heartbeat_stale hb#2.rx,node_stale,heartbeat_alive hb#2.rx,node_alive" ]
	[ "$(posts | tail -n 1)" = "{\"event\":\"node_alive\",\"cluster\":\"lab\",\"node\":\"$host\",\"peer\":\"site-b\",\"text\":\"$host: site-b beats again\"}" ]
}

@test "the streams of a daemon started less than 30 s ago are not judged, its arbitrators are" {
	started=$(date -u -d '10 seconds ago' +%Y-%m-%dT%H:%M:%S.%NZ)
	hb1=false
	hb2=false
	arbitrator=down
	check
	[ "$(events)" = "arbitrator_down" ]
}

@test "the check stays quiet while om3 does not answer" {
	hb2=false
	om_reports
	printf 'not json' >"$OM_STATUS"
	run sh "$SCRIPT" --check
	[ "$status" -eq 0 ]
	[ ! -e "$POSTS" ]
	chmod -x "$fake/om"
	run sh "$SCRIPT" --check
	[ "$status" -eq 0 ]
	[ ! -e "$POSTS" ]
}

@test "two checks at once alert once" {
	hb2=false
	om_reports
	OM_DELAY=0.5 sh "$SCRIPT" --check &
	OM_DELAY=0.5 sh "$SCRIPT" --check
	wait
	[ "$(events)" = "heartbeat_stale hb#2.rx" ]
}

@test "the om3 hook returns at once and reconciles in the background" {
	hb2=false
	om_reports
	start=$(date +%s%N)
	OM_DELAY=1 EVENT='{"kind":"HeartbeatStale","data":{}}' sh "$SCRIPT"
	[ $((($(date +%s%N) - start) / 1000000)) -lt 500 ]
	for _ in $(seq 50); do
		[ -s "$POSTS" ] && break
		sleep 0.1
	done
	[ "$(events)" = "heartbeat_stale hb#2.rx" ]
}

@test "every split action alerts at once, crit in the journal first, with a webhook timeout inside the 1.7 s before the reboot" {
	split='{"kind":"NodeSplitAction","data":{"node":"site-b","action":"reboot","node_votes":1,"arbitrator_votes":0,"voting":3,"pro_voters":1}}'
	EVENT=$split sh "$SCRIPT" --split
	EVENT=$split sh "$SCRIPT" --split
	[ "$(wc -l <"$POSTS")" -eq 2 ]
	[ "$(posts | head -n 1)" = "{\"event\":\"split_action\",\"cluster\":\"lab\",\"node\":\"$host\",\"action\":\"reboot\",\"pro_voters\":1,\"voting\":3,\"text\":\"$host: cluster split without quorum, 1 of 3 votes, split action reboot\"}" ]
	[ "$(head -n 1 "$LOG")" = "-t maintenant-cluster -p user.crit -- event=split_action cluster=lab node=$host action=reboot pro_voters=1 voting=3" ]
	[ "$(head -n 1 "$CURL_ARGS")" = "-fsS -m 1.5 -o /dev/null -H Content-Type: application/json --data-binary @- http://receiver.test/" ]
}

@test "the split hook ignores any other event" {
	EVENT='{"kind":"AuditStart","data":{"subsystems":["hook"]}}' sh "$SCRIPT" --split
	EVENT='not json' sh "$SCRIPT" --split
	[ ! -e "$POSTS" ]
	[ ! -e "$LOG" ]
}

@test "a failed webhook is logged and never fails the hook" {
	hb2=false
	om_reports
	CURL_FAILS=1 run sh "$SCRIPT" --check
	[ "$status" -eq 0 ]
	[ "$(tail -n 1 "$LOG")" = "-t maintenant-cluster -p user.warning -- event=webhook_failed alert=heartbeat_stale rc=7" ]
}

@test "without a webhook the alert only goes to the journal" {
	printf 'state_dir=%s\n' "$state" >"$CLUSTER_ALERT_CONF"
	hb2=false
	check
	[ ! -e "$CURL_ARGS" ]
	[ "$(cat "$LOG")" = "-t maintenant-cluster -p user.warning -- event=heartbeat_stale cluster= node=$host peer=site-b hb=hb#2.rx" ]
}
