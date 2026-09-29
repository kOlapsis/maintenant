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
if local_running; then
	stop_local
fi

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

if [ "$full" = yes ]; then
	method=basebackup
	log_event "rebuild_started method=basebackup"
	peer_sql "select pg_drop_replication_slot(slot_name) from pg_replication_slots where slot_name = '$HA_SELF_NAME' and not active" >/dev/null
	find "$PG_DATADIR" -mindepth 1 -delete
	as_postgres "$PG_BINDIR/pg_basebackup" -w -h "$HA_PEER_ADDRESS" -p "$PG_PORT" -U "$HA_USER" \
		-D "$PG_DATADIR" -X stream -C -S "$HA_SELF_NAME" --checkpoint=fast
fi

as_postgres touch "$PG_DATADIR/standby.signal"
rm -f "$HA_FENCED"
if ! start_local; then
	mark_fenced "the rebuilt standby did not start"
	exit 1
fi
if [ "$(local_sql 'select pg_is_in_recovery()')" != t ]; then
	stop_local
	mark_fenced "the rebuilt node came up as a primary"
	exit 1
fi
log_event "rebuilt method=$method"
