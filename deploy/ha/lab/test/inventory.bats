#!/usr/bin/env bats

INVENTORY="$BATS_TEST_DIRNAME/../inventory"
ROOT=/var/lib/maintenant

@test "a file written outside the state root fails the verdict and is named" {
	run sh "$INVENTORY" verdict "$ROOT" <<-'EOF'
	site-a	/var/lib/maintenant/maintenant.db
	site-a	/var/lib/maintenant/shm
	site-b	/var/tmp/lab-parasite
	site-b	/var/lib/maintenant-agent/health
	EOF
	[ "$status" -eq 0 ]
	[ "$(jq -r '.verdict' <<<"$output")" = fail ]
	[ "$(jq -c '.outside_volume' <<<"$output")" = '["site-b:/var/lib/maintenant-agent/health","site-b:/var/tmp/lab-parasite"]' ]
	[ "$(jq -c '.written["site-a"]' <<<"$output")" = '["/var/lib/maintenant/maintenant.db","/var/lib/maintenant/shm"]' ]
}

@test "the state root, /dev/null and the pseudo file systems pass, /run and /tmp do not" {
	run sh "$INVENTORY" verdict "$ROOT" <<-'EOF'
	site-a	/var/lib/maintenant
	site-a	/var/lib/maintenant/tmp/etilqs_1
	site-a	/dev/null
	site-a	/proc/self/oom_score_adj
	site-a	/sys/fs/cgroup/x
	EOF
	[ "$(jq -r '.verdict' <<<"$output")" = pass ]
	run sh "$INVENTORY" verdict "$ROOT" <<-'EOF'
	site-a	/run/maintenant.pid
	site-a	/tmp/x
	EOF
	[ "$(jq -c '.outside_volume' <<<"$output")" = '["site-a:/run/maintenant.pid","site-a:/tmp/x"]' ]
}

@test "the trace keeps the writes of the window and drops the reads" {
	mkdir "$BATS_TEST_TMPDIR/trace"
	cat >"$BATS_TEST_TMPDIR/trace/1-0.log" <<-'EOF'
	1794953999.000000 maintenant(10): W   /tmp/before
	1794954000.000001 maintenant(10): O   /var/lib/maintenant/maintenant.db
	1794954000.000002 maintenant(10): R   /etc/ssl/certs/ca-certificates.crt
	1794954000.000003 maintenant(10): CW  /var/lib/maintenant/.maintenant-license
	1794954000.000004 maintenant(10): +   /var/lib/maintenant/tmp
	1794954000.000005 maintenant(10): C   /etc/hosts
	1794954000.000006 maintenant(10): <>  /var/lib/maintenant/shm
	1794954000.000007 maintenant(10): W   /var/tmp/a file
	1794954000.000008 maintenant(10): CWO (deleted)
	1794954000.000009 maintenant(10): W   /var/lib/maintenant/tmp/etilqs_1 (deleted)
	EOF
	cat >"$BATS_TEST_TMPDIR/trace/1-1.log" <<-'EOF'
	1794954000.500000 maintenant(10): CWO /run/lab-parasite
	1794954002.000000 maintenant(10): W   /tmp/after
	EOF
	run sh "$INVENTORY" trace "$BATS_TEST_TMPDIR/trace" 1794954000 1794954001
	[ "$status" -eq 0 ]
	[ "$output" = "/run/lab-parasite
/var/lib/maintenant/.maintenant-license
/var/lib/maintenant/shm
/var/lib/maintenant/tmp
/var/lib/maintenant/tmp/etilqs_1
/var/tmp/a file" ]
}

@test "a missing trace directory is an error, not an empty trace" {
	run sh "$INVENTORY" trace "$BATS_TEST_TMPDIR/none" 0 1
	[ "$status" -ne 0 ]
}

lab_functions() {
	sed '$d' "$BATS_TEST_DIRNAME/../lab" >"$BATS_TEST_TMPDIR/lab.sh"
	cat >>"$BATS_TEST_TMPDIR/lab.sh" <<-EOF
	LAB_DIR="$BATS_TEST_DIRNAME/.."
	group_hosts() { printf 'site-a\nsite-b\n'; }
	ssh_to() { cat "$BATS_TEST_TMPDIR/trace-\$1"; }
	EOF
}

measure() {
	lab_functions
	printf '%s\n' "$1" >"$BATS_TEST_TMPDIR/before"
	printf '%s\n' "$2" >"$BATS_TEST_TMPDIR/after"
	sh -c ". '$BATS_TEST_TMPDIR/lab.sh'; inventory_measure '$BATS_TEST_TMPDIR/before' '$BATS_TEST_TMPDIR/after' 0 1"
}

@test "a trace restarted on a node that stayed up is not measured, never a pass" {
	printf '/var/lib/maintenant/maintenant.db\n' >"$BATS_TEST_TMPDIR/trace-site-a"
	: >"$BATS_TEST_TMPDIR/trace-site-b"
	run measure "site-a active i1 a1
site-b active j1 b1" "site-a active i1 a1
site-b active j2 b1"
	[ "$status" -eq 0 ]
	[ "$(jq -r '.verdict' <<<"$output")" = not_measured ]
	[ "$(jq -c '.reasons' <<<"$output")" = '["the trace restarted on site-b while the node stayed up"]' ]
}

@test "a node that rebooted keeps a measured trace, and a parasite fails it even with a gap" {
	printf '/var/lib/maintenant/maintenant.db\n' >"$BATS_TEST_TMPDIR/trace-site-a"
	: >"$BATS_TEST_TMPDIR/trace-site-b"
	run measure "site-a active i1 a1
site-b active j1 b1" "site-a active i2 a2
site-b active j1 b1"
	[ "$(jq -r '.verdict' <<<"$output")" = pass ]
	printf '/var/tmp/lab-parasite\n' >"$BATS_TEST_TMPDIR/trace-site-b"
	run measure "site-a active i1 a1
site-b inactive - b1" "site-a active i1 a1
site-b active j1 b1"
	[ "$(jq -r '.verdict' <<<"$output")" = fail ]
	[ "$(jq -c '.outside_volume' <<<"$output")" = '["site-b:/var/tmp/lab-parasite"]' ]
	[ "$(jq -c '.reasons' <<<"$output")" = '["the trace did not run on site-b at the start"]' ]
}
