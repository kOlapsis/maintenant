def us_s: ((. / 1000) | round) / 1000;
def iso_us: capture("^(?<s>[0-9-]+T[0-9:]+)(\\.(?<f>[0-9]+))?Z$")
	| ((.s + "Z") | fromdateiso8601) * 1000000 + (((.f // "") + "000000")[0:6] | tonumber);
def parsed: (.msg | [scan("([a-z_]+)=(\"[^\"]*\"|[^ ]+)")]
		| map({key: .[0], value: (.[1] | ltrimstr("\"") | rtrimstr("\""))}) | from_entries) as $kv
	| {node, ts, tag, boot, msg, event: $kv.event, kv: $kv};
def svc($e): .tag == "maintenant-svc" and .event == $e;
def pg($e): .tag == "maintenant-pg" and .event == $e;
def fence($e): .tag == "maintenant-fence" and .event == $e;
def drbd_role($to): .tag == "kernel" and (.msg | test("^drbd maintenant: (.* )?role\\( [A-Za-z]+ -> \($to) \\)"));
def kernel_drbd($re): .tag == "kernel" and (.msg | test($re));
def split_brain: kernel_drbd("Split-Brain detected");
def takes_service: svc("started") or pg("promoting") or pg("promoted") or pg("primary_started") or drbd_role("Primary");
def drbd: $mode == "sqlite/drbd";

$nodes_file[0] as $nodes
| ($nodes | to_entries | map(select(.value.readable | not) | .key)) as $unreadable
| ($nodes | to_entries | map(select(.value.boot_before == "" or .value.boot_after == "") | .key)) as $unbooted
| ($nodes | to_entries | map(select(.value.boot_before != "" and .value.boot_after != ""
	and .value.boot_before != .value.boot_after) | .key)) as $crashed
| ([$nodes | to_entries[] | .key as $n | .value.events[] | . + {node: $n} | parsed
	| select(.ts >= $t0 and .ts <= $end)] | sort_by(.ts)) as $ev
| (if ($unreadable | length) > 0 then "the journal of \($unreadable | join(", ")) could not be read" else null end) as $journal_error
| (if ($unbooted | length) > 0 then "the boot id of \($unbooted | join(", ")) could not be read" else null end) as $boot_error

| def local_restarts:
	if $restart == null then {error: "the scenario names no local_restart to check"}
	else ($restart.resource // "app") as $res
	| ($settings.local_restarts[$res]? // null) as $limit
	| ($restart.failover // false) as $expected
	| if ($expected | not) and $limit == null then {error: "the restart limit of \($res) could not be read from the settings"}
	  else {value: {
		resource: $res, node: $holder, limit: $limit, failover_expected: $expected,
		count: ([$ev[] | select(.node == $holder)
			| select(if $res == "postgres" then pg("primary_started") or pg("primary_start_failed")
				else svc("started") or svc("start_failed") end)] | length),
		failover: any($ev[]; .node != $holder and (svc("started") or pg("promoted")))
	  }} end
	end;

def failover_count:
	{value: (reduce ($ev[] | select(svc("started") or svc("start_failed"))) as $s ({at: $holder, n: 0};
		if $s.node != .at then {at: $s.node, n: (.n + 1)} else . end) | .n)};

def detection_delay_s:
	([$ev[] | select(.node == $holder and svc("check_dead"))] | first) as $d
	| if $d == null then {error: "no check_dead logged on \($holder) during the repetition"}
	  else {value: (($d.ts - $t0) | us_s)} end;

def fencing_outcome:
	([$ev[] | select(.node != $holder and takes_service)] | first) as $s
	| {value: {
		holder: $holder,
		fenced: ([$ev[] | select(fence("fenced")) | .kv.target] | unique),
		failed: ([$ev[] | select(fence("fence_failed")) | .kv.target] | unique),
		started_elsewhere: ($s != null),
		started_elsewhere_before_fence: ($s != null
			and ([$ev[] | select(fence("fenced") and .kv.target == $holder and .ts <= $s.ts)] | length) == 0)
	}};

def agent_watchdog_restart:
	[$ev[] | select(.node == $holder and .tag == "systemd")] as $sd
	| ([$sd[] | select(.msg | startswith("opensvc-server.service: Watchdog timeout"))] | first) as $w
	| {value: (if $w == null then 0
		else [$sd[] | select(.ts >= $w.ts and (.msg | startswith("opensvc-server.service: Scheduled restart job")))] | length end)};

def primary_count:
	if $post == null then {error: "the status after the repair could not be read"}
	else ([$post.nodes[] | select(.postgres.role? == "primary") | .name]) as $p
	| {value: {count: ($p | length), nodes: $p, two_primaries: ([$ev[] | select(pg("two_primaries"))] | length)}}
	end;

def former_primary_rebuild:
	if $post == null then {error: "the status after the repair could not be read"}
	else ([$post.nodes[] | select(.postgres.role? == "standby") | .name]) as $sb
	| if ($sb | length) != 1 then {error: "\($sb | length) standbys after the repair, expected 1"}
	  else $sb[0] as $n
	  | ($nodes[$n].boot_before // "") as $boot0
	  | if $boot0 == "" then {error: "the boot id of \($n) could not be read"}
	    else [$ev[] | select(.node == $n and .tag == "maintenant-pg")] as $ne
	    | ([$ne[] | select(.event == "rebuilt")] | last) as $done
	    | ([$ne[] | select(.event == "primary_stopped" or .event == "fenced")] | first) as $lost
	    | ([$ne[] | select(.event == "rebuild_started" and ($done == null or .ts <= $done.ts))] | first) as $began
	    | {value: {
		node: $n,
		rebuilt: ($done != null),
		method: $done.kv.method,
		duration_s: (if $done == null or $began == null then null else (($done.ts - $began.ts) | us_s) end),
		primary_started_before_rebuild: any($ne[];
			(.event == "primary_started" or .event == "primary_confirmed" or .event == "promoted")
			and ($done == null or .ts < $done.ts)
			and (.boot != $boot0 or ($lost != null and .ts > $lost.ts)))
	    }} end
	  end
	end;

def sync_standby:
	if ($settings.sync_standby? // null) == null then {error: "settings.sync_standby could not be read"}
	else {value: $settings.sync_standby} end;

def loss_window:
	([$ev[] | select(pg("sync_dropped"))]) as $open
	| ($open | first) as $o
	| (if $o == null then null else ([$ev[] | select(pg("sync_restored") and .ts >= $o.ts)] | first) end) as $c
	| {value: {
		conduct: ($settings.sync_standby? // null),
		opened: ($o != null),
		closed: ($c != null),
		duration_s: (if $c == null then null else (($c.ts - $o.ts) | us_s) end),
		reasons: ([$open[].kv.reason] | unique)
	}};

def crash_confirmed:
	if $boot_error != null then {error: $boot_error}
	elif ([$crashed[] | select($nodes[.].previous_boot_readable | not)] | length) > 0 then
		{error: "the journal of \([$crashed[] | select($nodes[.].previous_boot_readable | not)] | join(", ")) from before the crash is gone"}
	else {value: {
		crashed: $crashed,
		started_before_crash: ([$crashed[] as $n | $ev[] | select(.node == $n and .boot == $nodes[$n].boot_before and takes_service) | $n] | unique),
		confirmed: (($crashed | length) > 0
			and ([$crashed[] as $n | $ev[] | select(.node == $n and .boot == $nodes[$n].boot_before and takes_service)] | length) == 0)
	}} end;

def overlap(up; down):
	if $boot_error != null then {error: $boot_error}
	elif ([$crashed[] | select($nodes[.].btime_us == null)] | length) > 0 then
		{error: "the boot time of \([$crashed[] | select($nodes[.].btime_us == null)] | join(", ")) could not be read"}
	else
		([$ev[] | select(up) | {ts, node, up: true}]
		 + [$ev[] | select(down) | {ts, node, up: false}]
		 + [$ev[] | select(fence("fenced")) | {ts, node: .kv.target, up: false}]
		 + [$crashed[] | {ts: $nodes[.].btime_us, node: ., up: false}])
		| sort_by(.ts)
		| {value: (reduce .[] as $e ({run: {($holder): true}, max: 1, overlaps: 0};
			([.run[] | select(.)] | length) as $was
			| .run[$e.node] = $e.up
			| ([.run[] | select(.)] | length) as $now
			| .max = ([.max, $now] | max)
			| .overlaps += (if $was < 2 and $now >= 2 then 1 else 0 end)))}
	end;

def app_overlap: overlap(svc("started"); svc("stopped") or svc("start_failed")) | if .value == null then . else {value: .value.max} end;

def drbd_overlap: overlap(drbd_role("Primary"); drbd_role("Secondary"));

def drbd_primary_count:
	if $post == null then {error: "the status after the repair could not be read"}
	else ([$post.nodes[] | select(.drbd.role? == "Primary") | .name]) as $p
	| drbd_overlap | if .value == null then . else {value: {count: ($p | length), nodes: $p, two_primaries: .value.overlaps}} end
	end;

def drbd_rebuild:
	if $post == null then {error: "the status after the repair could not be read"}
	else ([$post.nodes[] | select(.drbd.role? == "Secondary")]) as $sb
	| if ($sb | length) != 1 then {error: "\($sb | length) Secondaries after the repair, expected 1"}
	  else $sb[0].name as $n
	  | ($nodes[$n].boot_before // "") as $boot0
	  | if $boot0 == "" then {error: "the boot id of \($n) could not be read"}
	    else [$ev[] | select(.node == $n and .tag == "kernel")] as $ne
	    | ([$ne[] | select(kernel_drbd("repl\\( SyncTarget -> Established \\)"))] | last) as $done
	    | ([$ne[] | select(drbd_role("Secondary"))] | first) as $lost
	    | ([$ne[] | select(kernel_drbd("repl\\( [A-Za-z]+ -> SyncTarget \\)") and ($done == null or .ts <= $done.ts))] | first) as $began
	    | {value: {
		node: $n,
		rebuilt: ($sb[0].drbd.disk == "UpToDate"),
		method: (if $done == null then null else "resync" end),
		duration_s: (if $done == null or $began == null then null else (($done.ts - $began.ts) | us_s) end),
		primary_started_before_rebuild: any($ne[];
			drbd_role("Primary")
			and ($done == null or .ts < $done.ts)
			and (.boot != $boot0 or ($lost != null and .ts > $lost.ts)))
	    }} end
	  end
	end;

def forks:
	[$ev[] | select(pg("promoted")) as $p
		| ([$ev[] | select(.node == $p.node and pg("promoting") and .ts <= $p.ts)] | last // $p) as $began
		| ($nodes | keys[] | select(. != $p.node)) as $f
		| {promoted: $p.node, at: $began.ts, former: $f,
		   report: ([$ev[] | select(.node == $f and pg("post_fork_commits") and .ts > $p.ts)] | first)}];

def after_promotion:
	forks as $forks
	| ([$forks[] | select(.report == null) | "\(.former) logged no post_fork_commits after the promotion of \(.promoted)"]
	   + [$forks[] | select(.report != null and (.report.kv.count | tonumber? // null) == null)
		| "the WAL of \(.former) past the fork could not be read"]) as $missing
	| if ($missing | length) > 0 then {error: ($missing | join("; "))}
	  else {value: ([$forks[] | (.report.kv.count | tonumber) as $count | .report.kv.last_commit as $last
		| if $count > 0 and $last != "none" and ($last | iso_us) > .at then $count else 0 end] | add // 0)}
	  end;

def nodes_written:
	if $mode == "ip-only" then app_overlap
	elif drbd then drbd_overlap | if .value == null then . else {value: (if .value.max >= 2 or any($ev[]; split_brain) then 2 else 1 end)} end
	else after_promotion | if .value == null then . else {value: (if .value > 0 then 2 else 1 end)} end
	end;

def measure($m):
	if ($not_applicable | index($m)) != null then {value: "not_applicable"}
	elif $m == "sync_standby" then sync_standby
	elif $m == "primary_count" and (drbd | not) then primary_count
	elif $journal_error != null then {error: $journal_error}
	elif $m == "primary_count" then drbd_primary_count
	elif $m == "local_restarts" then local_restarts
	elif $m == "failover_count" then failover_count
	elif $m == "detection_delay_s" then detection_delay_s
	elif $m == "fencing_outcome" then fencing_outcome
	elif $m == "agent_watchdog_restart" then agent_watchdog_restart
	elif $m == "former_primary_rebuild" then (if drbd then drbd_rebuild else former_primary_rebuild end)
	elif $m == "loss_window" then loss_window
	elif $m == "crash_confirmed" then crash_confirmed
	elif $m == "nodes_written" then nodes_written
	elif $m == "divergence" then (if drbd then {value: (if any($ev[]; split_brain) then 1 else 0 end)} else after_promotion end)
	else {error: "the bench has no rule for \($m)"} end;

[$declared[] as $m | measure($m) | {m: $m} + .] as $results
| ([$results[] | {key: .m, value: (.value // null)}] | from_entries)
  + {errors: ([$results[] | select(.error != null) | {key: .m, value: .error}] | from_entries)}
