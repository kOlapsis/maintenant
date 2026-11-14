#!/usr/bin/env bats

MEASURES="$BATS_TEST_DIRNAME/../node-measures.jq"
DECLARED='["primary_count", "nodes_written", "divergence", "former_primary_rebuild", "crash_confirmed", "fencing_outcome", "failover_count", "sync_standby", "loss_window"]'
T0="2026-10-27T19:54:40.000000Z"
END="2026-10-27T20:01:00.000000Z"

us() {
	jq -nr --arg t "$1" '$t | capture("^(?<s>[^.]+)\\.(?<f>[0-9]{6})Z$") | ((.s + "Z") | fromdateiso8601) * 1000000 + (.f | tonumber)'
}

# Reads "node<TAB>time<TAB>tag<TAB>boot<TAB>message" journal lines; $1 maps each node to its boot ids and boot time.
journal() {
	jq -Rn --argjson meta "$1" '
		def us: capture("^(?<s>[^.]+)\\.(?<f>[0-9]{6})Z$") | ((.s + "Z") | fromdateiso8601) * 1000000 + (.f | tonumber);
		[inputs | select(length > 0) | split("\t") | {node: .[0], ts: (.[1] | us), tag: .[2], boot: .[3], msg: .[4]}] as $lines
		| $meta | with_entries(.key as $n | .value += {readable: true, previous_boot_readable: true,
			events: [$lines[] | select(.node == $n) | del(.node)]})'
}

# post <role of site-a> <disk of site-a> <role of site-b> <disk of site-b>
post() {
	jq -nc --arg ar "$1" --arg ad "$2" --arg br "$3" --arg bd "$4" \
		'{nodes: [{name: "site-a", drbd: {role: $ar, disk: $ad}}, {name: "site-b", drbd: {role: $br, disk: $bd}}]}'
}

measures() {
	journal "$1" >"$BATS_TEST_TMPDIR/nodes.json"
	jq -c \
		--argjson declared "$DECLARED" \
		--arg mode sqlite/drbd \
		--slurpfile nodes_file "$BATS_TEST_TMPDIR/nodes.json" \
		--arg holder site-a \
		--argjson t0 "$(us "$T0")" \
		--argjson end "$(us "$END")" \
		--argjson post "$2" \
		--argjson restart null \
		--argjson settings '{"local_restarts": {"app": 2}}' \
		--argjson not_applicable '["sync_standby", "loss_window"]' \
		-n -f "$MEASURES"
}

CRASHED_A='{"site-a": {"boot_before": "a1", "boot_after": "a2", "btime_us": 1793131000000000}, "site-b": {"boot_before": "b1", "boot_after": "b1", "btime_us": null}}'
NO_CRASH='{"site-a": {"boot_before": "a1", "boot_after": "a1", "btime_us": null}, "site-b": {"boot_before": "b1", "boot_after": "b1", "btime_us": null}}'

@test "a clean failover: one primary, one node written, the returning node resynced as Secondary" {
	run measures "$CRASHED_A" "$(post Secondary UpToDate Primary UpToDate)" <<-'EOF'
	site-b	2026-10-27T19:54:46.320011Z	kernel	b1	drbd maintenant site-a: conn( Connected -> NetworkFailure ) peer( Primary -> Unknown )
	site-b	2026-10-27T19:54:54.829116Z	maintenant-fence	b1	event=fenced target=site-a
	site-b	2026-10-27T19:54:59.995239Z	kernel	b1	drbd maintenant: Preparing cluster-wide state change 659871228: 1->all role( Primary )
	site-b	2026-10-27T19:54:59.999545Z	kernel	b1	drbd maintenant: role( Secondary -> Primary ) [primary]
	site-b	2026-10-27T19:55:01.493206Z	maintenant-svc	b1	event=started
	site-a	2026-10-27T19:57:14.065520Z	kernel	a2	drbd maintenant/0 drbd0: disk( Attaching -> Consistent ) [attach]
	site-a	2026-10-27T19:57:14.881067Z	kernel	a2	drbd maintenant/0 drbd0: disk( Consistent -> Inconsistent ) [receive-bitmap]
	site-a	2026-10-27T19:57:14.881106Z	kernel	a2	drbd maintenant/0 drbd0 site-b: repl( WFBitMapT -> SyncTarget ) [receive-bitmap]
	site-a	2026-10-27T19:57:14.881115Z	kernel	a2	drbd maintenant/0 drbd0 site-b: Began resync as SyncTarget (will sync 5062656 KB [1265664 bits set]).
	site-a	2026-10-27T19:58:27.540261Z	kernel	a2	drbd maintenant/0 drbd0 site-b: Resync done (total 72 sec; paused 0 sec; 70312 K/sec)
	site-a	2026-10-27T19:58:27.540359Z	kernel	a2	drbd maintenant/0 drbd0: disk( Inconsistent -> UpToDate ) [resync-finished]
	site-a	2026-10-27T19:58:27.540378Z	kernel	a2	drbd maintenant/0 drbd0 site-b: repl( SyncTarget -> Established ) [resync-finished]
	EOF
	[ "$status" -eq 0 ]
	[ "$(jq -c '.primary_count' <<<"$output")" = '{"count":1,"nodes":["site-b"],"two_primaries":0}' ]
	[ "$(jq -c '.nodes_written' <<<"$output")" = 1 ]
	[ "$(jq -c '.divergence' <<<"$output")" = 0 ]
	[ "$(jq -c '.former_primary_rebuild' <<<"$output")" = '{"node":"site-a","rebuilt":true,"method":"resync","duration_s":72.659,"primary_started_before_rebuild":false}' ]
	[ "$(jq -c '.crash_confirmed' <<<"$output")" = '{"crashed":["site-a"],"started_before_crash":[],"confirmed":true}' ]
	[ "$(jq -c '.fencing_outcome.started_elsewhere_before_fence' <<<"$output")" = false ]
	[ "$(jq -c '.failover_count' <<<"$output")" = 1 ]
	[ "$(jq -c '[.sync_standby, .loss_window]' <<<"$output")" = '["not_applicable","not_applicable"]' ]
	[ "$(jq -c '.errors' <<<"$output")" = '{}' ]
}

@test "two primaries at once count once and mean both nodes wrote" {
	run measures "$NO_CRASH" "$(post Primary UpToDate Secondary UpToDate)" <<-'EOF'
	site-b	2026-10-27T19:54:41.100000Z	kernel	b1	drbd maintenant site-a: conn( Connected -> Disconnecting ) peer( Primary -> Unknown )
	site-b	2026-10-27T19:54:41.300000Z	kernel	b1	drbd maintenant: role( Secondary -> Primary ) [primary]
	site-b	2026-10-27T19:54:42.900000Z	kernel	b1	drbd maintenant: role( Primary -> Secondary ) [secondary]
	EOF
	[ "$status" -eq 0 ]
	[ "$(jq -c '.primary_count' <<<"$output")" = '{"count":1,"nodes":["site-a"],"two_primaries":1}' ]
	[ "$(jq -c '.nodes_written' <<<"$output")" = 2 ]
}

@test "a split brain DRBD logged means both nodes wrote and the data diverged" {
	run measures "$NO_CRASH" "$(post Primary UpToDate Secondary UpToDate)" <<-'EOF'
	site-a	2026-10-27T19:54:44.200000Z	kernel	a1	drbd maintenant/0 drbd0 site-b: drbd_sync_handshake:
	site-a	2026-10-27T19:54:44.200100Z	kernel	a1	drbd maintenant/0 drbd0 site-b: Split-Brain detected but unresolved, dropping connection!
	site-a	2026-10-27T19:54:44.200200Z	kernel	a1	drbd maintenant site-b: conn( Connected -> Disconnecting )
	EOF
	[ "$status" -eq 0 ]
	[ "$(jq -c '.primary_count.two_primaries' <<<"$output")" = 0 ]
	[ "$(jq -c '.nodes_written' <<<"$output")" = 2 ]
	[ "$(jq -c '.divergence' <<<"$output")" = 1 ]
}

@test "a returning node promoted before its resync finished is caught" {
	run measures "$CRASHED_A" "$(post Secondary UpToDate Primary UpToDate)" <<-'EOF'
	site-b	2026-10-27T19:54:54.829116Z	maintenant-fence	b1	event=fenced target=site-a
	site-b	2026-10-27T19:54:59.999545Z	kernel	b1	drbd maintenant: role( Secondary -> Primary ) [primary]
	site-a	2026-10-27T19:57:14.065520Z	kernel	a2	drbd maintenant/0 drbd0: disk( Attaching -> Consistent ) [attach]
	site-a	2026-10-27T19:57:15.067065Z	kernel	a2	drbd maintenant: role( Secondary -> Primary ) [primary]
	site-a	2026-10-27T19:57:18.100000Z	kernel	a2	drbd maintenant: role( Primary -> Secondary ) [secondary]
	site-a	2026-10-27T19:57:20.881106Z	kernel	a2	drbd maintenant/0 drbd0 site-b: repl( WFBitMapT -> SyncTarget ) [receive-bitmap]
	site-a	2026-10-27T19:57:30.540378Z	kernel	a2	drbd maintenant/0 drbd0 site-b: repl( SyncTarget -> Established ) [resync-finished]
	EOF
	[ "$status" -eq 0 ]
	[ "$(jq -c '.former_primary_rebuild.primary_started_before_rebuild' <<<"$output")" = true ]
	[ "$(jq -c '.primary_count.two_primaries' <<<"$output")" = 1 ]
}

@test "a returning node that is not UpToDate after the repair is not rebuilt" {
	run measures "$CRASHED_A" "$(post Secondary Inconsistent Primary UpToDate)" <<-'EOF'
	site-b	2026-10-27T19:54:54.829116Z	maintenant-fence	b1	event=fenced target=site-a
	site-b	2026-10-27T19:54:59.999545Z	kernel	b1	drbd maintenant: role( Secondary -> Primary ) [primary]
	site-a	2026-10-27T19:57:20.881106Z	kernel	a2	drbd maintenant/0 drbd0 site-b: repl( WFBitMapT -> SyncTarget ) [receive-bitmap]
	EOF
	[ "$status" -eq 0 ]
	[ "$(jq -c '.former_primary_rebuild' <<<"$output")" = '{"node":"site-a","rebuilt":false,"method":null,"duration_s":null,"primary_started_before_rebuild":false}' ]
}
