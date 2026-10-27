#!/bin/sh
# Rebuilds this node as a standby of the peer: pg_rewind first, a full
# pg_basebackup when the rewind fails, when --full is asked, or when the peer
# no longer holds the WAL this node would need.
#
#   maintenant-pg-rebuild [--full]

set -eu

# shellcheck source-path=SCRIPTDIR source=common.sh
. /usr/local/lib/maintenant-pg/common.sh

full=no
if [ "${1:-}" = --full ]; then
	full=yes
elif [ $# -gt 0 ]; then
	echo "usage: $0 [--full]" >&2
	exit 2
fi

post_fork_commits() {
	segment=""
	for wal in "$PG_DATADIR"/pg_wal/[0-9A-F]???????????????????????; do
		[ -f "$wal" ] && segment=${wal##*/}
	done
	[ -n "$segment" ] || return 1
	tli=$(printf '%s' "$segment" | cut -c1-8)
	peer_wal=$(peer_sql 'select pg_walfile_name(pg_current_wal_lsn())') || return 1
	peer_tli=$(printf '%s' "$peer_wal" | cut -c1-8)
	if [ "$peer_tli" = "$tli" ]; then
		echo "count=0 first_commit=none last_commit=none timeline=$tli"
		return 0
	fi
	history=$(peer_sql "select convert_from(pg_read_binary_file('pg_wal/$peer_tli.history'), 'UTF8')") || return 1
	fork=$(printf '%s\n' "$history" | awk -v t="$((0x$tli))" '$1 == t { print $2 }')
	[ -n "$fork" ] || return 1
	segsize=$(peer_sql "select setting from pg_settings where name = 'wal_segment_size'") || return 1
	lsn=$(((0x${fork%/*} << 32) | 0x${fork#*/}))
	per_id=$((0x100000000 / segsize))
	startseg=$(printf '%s%08X%08X' "$tli" $((lsn / segsize / per_id)) $((lsn / segsize % per_id)))
	# Without a start segment, pg_waldump reads the segment size from any file, a preallocated empty one included.
	# pg_waldump fails at the end of the WAL whatever happens: only a read error counts.
	dump=$(as_postgres env TZ=UTC "$PG_BINDIR/pg_waldump" --path="$PG_DATADIR/pg_wal" \
		--start="$fork" --rmgr=Transaction "$startseg" 2>"$HA_RUN_DIR/waldump.err") ||
		grep -q -e 'error in WAL record at' -e 'could not find a valid record after' "$HA_RUN_DIR/waldump.err" ||
		return 1
	commits=$(printf '%s\n' "$dump" | sed -n 's/.*desc: COMMIT \([0-9-]*\) \([0-9:.]*\) UTC.*/\1T\2Z/p')
	count=$(printf '%s' "$commits" | grep -c . || true)
	first=none
	last=none
	if [ "$count" -gt 0 ]; then
		first=$(printf '%s\n' "$commits" | head -n 1)
		last=$(printf '%s\n' "$commits" | tail -n 1)
	fi
	echo "count=$count first_commit=$first last_commit=$last timeline=$tli fork_lsn=$fork"
}

start_standby() {
	as_postgres touch "$PG_DATADIR/standby.signal"
	rm -f "$HA_FENCED"
	start_local
}

mkdir -p "$HA_RUN_DIR"
exec 9>"$HA_RUN_DIR/rebuild.lock"
if ! flock -n 9; then
	echo "a rebuild is already running on this node" >&2
	exit 1
fi

if [ "$(peer_role)" != primary ]; then
	echo "$HA_PEER is not a primary: there is nothing to rebuild from" >&2
	exit 1
fi

if [ ! -f "$HA_FENCED" ]; then
	mark_fenced "rebuild requested"
fi
# A standby still starting refuses connections: only the unit tells it is running.
stop_local

fork_report=$(post_fork_commits) || fork_report="count=unknown"
log_event "post_fork_commits $fork_report"

slot=$(own_slot_on_peer)
if [ "$full" = no ] && [ "$slot" != ok ]; then
	log_event "rewind_skipped slot=$slot"
	full=yes
fi

method=rewind
if [ "$full" = no ]; then
	log_event "rebuild_started method=rewind"
	# Single-user crash recovery, which pg_rewind runs on an unclean target, refuses standby.signal.
	rm -f "$PG_DATADIR/standby.signal"
	if ! as_postgres "$PG_BINDIR/pg_rewind" \
		--target-pgdata="$PG_DATADIR" \
		--source-server="host=$HA_PEER_ADDRESS port=$PG_PORT user=$HA_USER dbname=postgres connect_timeout=5" \
		--config-file="$PG_CONFDIR/postgresql.conf"; then
		log_event "rewind_failed fallback=basebackup"
		full=yes
	fi
fi

if [ "$full" = no ] && ! start_standby; then
	stop_local
	mark_fenced "the rewound standby did not start"
	log_event "rewind_failed fallback=basebackup"
	full=yes
fi

if [ "$full" = yes ]; then
	method=basebackup
	log_event "rebuild_started method=basebackup"
	peer_sql "select pg_drop_replication_slot(slot_name) from pg_replication_slots where slot_name = '$HA_SELF_NAME' and not active" >/dev/null
	find "$PG_DATADIR" -mindepth 1 -delete
	as_postgres "$PG_BINDIR/pg_basebackup" -w -h "$HA_PEER_ADDRESS" -p "$PG_PORT" -U "$HA_USER" \
		-D "$PG_DATADIR" -X stream -C -S "$HA_SELF_NAME" --checkpoint=fast
	if ! start_standby; then
		stop_local
		mark_fenced "the rebuilt standby did not start"
		exit 1
	fi
fi

if [ "$(local_sql 'select pg_is_in_recovery()')" != t ]; then
	stop_local
	mark_fenced "the rebuilt node came up as a primary"
	exit 1
fi
log_event "rebuilt method=$method"
