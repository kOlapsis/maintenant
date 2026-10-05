#!/bin/sh
# Makes this node the one primary before the service starts here: promotes the
# standby, or clears a primary that stayed local. Any doubt exits non-zero, and
# the service does not start.

set -eu

# shellcheck source-path=SCRIPTDIR source=common.sh
. /usr/local/lib/maintenant-pg/common.sh

refuse() {
	log_event "promotion_refused reason=$1"
	printf 'refused: %s\n' "$2" >&2
	exit 1
}

wait_async() {
	tries=10
	until [ "$(local_sql "select current_setting('synchronous_standby_names') = ''")" = t ]; do
		tries=$((tries - 1))
		if [ "$tries" -le 0 ]; then
			return 1
		fi
		sleep 1
	done
}

wait_receiver_gone() {
	tries=10
	while [ "$(receiver_status)" = streaming ]; do
		tries=$((tries - 1))
		if [ "$tries" -le 0 ]; then
			return 1
		fi
		sleep 1
	done
}

promote_standby() {
	if [ ! -f "$HA_SYNCED" ]; then
		refuse never_synced "this standby has not caught up with a primary since it was built"
	fi
	if [ "$1" = standby-stopped ]; then
		start_local || refuse standby_start "the standby did not start"
	fi
	if ! wait_receiver_gone; then
		refuse peer_streaming "$HA_PEER still streams to this standby: it is alive as a primary"
	fi
	if [ "$(peer_role)" = primary ]; then
		refuse peer_primary "$HA_PEER answers as a primary"
	fi

	received=$(local_sql 'select pg_last_wal_receive_lsn()')
	log_event "promoting received_lsn=${received:-none} standby_missing=$HA_STANDBY_MISSING"
	drop_sync
	if ! wait_async; then
		restore_sync
		refuse sync_drop_failed "synchronous_standby_names is still set after the reload"
	fi
	allow_primary
	local_sql "select pg_promote(true, $HA_PROMOTE_TIMEOUT)" >/dev/null
	if [ "$(local_sql 'select pg_is_in_recovery()')" != f ]; then
		rm -f "$HA_ALLOWANCE"
		restore_sync
		refuse promote_failed "the standby is still in recovery after ${HA_PROMOTE_TIMEOUT}s"
	fi
	log_event "sync_dropped loss_window=open reason=promotion"

	local_sql "select pg_create_physical_replication_slot('$HA_PEER_NAME', true) where not exists (select 1 from pg_replication_slots where slot_name = '$HA_PEER_NAME')" >/dev/null
	position=$(local_sql 'select pg_walfile_name(pg_current_wal_lsn())')
	log_event "promoted walfile=$position"
}

keep_running_primary() {
	peer=$(peer_role)
	if [ "$peer" = primary ]; then
		stop_local
		refuse two_primaries "$HA_PEER is a primary too: the local primary is stopped, the operator decides which copy is kept"
	fi
	if [ ! -f "$HA_ALLOWANCE" ]; then
		if [ "$peer" != standby ]; then
			refuse unproven "a primary runs here without allowance and $HA_PEER cannot confirm it is a standby"
		fi
		allow_primary
		log_event "primary_confirmed peer=standby"
	fi
}

restart_stopped_primary() {
	peer=$(peer_role)
	if [ "$peer" = primary ]; then
		"$HA_SBIN_DIR/maintenant-pg-fence" "returned while $HA_PEER is primary"
		refuse overtaken "$HA_PEER was promoted while this node was away"
	fi
	if [ ! -f "$HA_ALLOWANCE" ]; then
		if [ "$peer" != standby ]; then
			refuse unproven "this data directory was a primary before the node restarted and $HA_PEER cannot confirm it is a standby: check which copy is newer, then create $HA_ALLOWANCE on the node to keep"
		fi
		allow_primary
		log_event "primary_confirmed peer=standby"
	fi
}

if [ -f "$HA_FENCED" ]; then
	refuse fenced "this node was overtaken and waits to be rebuilt as a standby"
fi

state=$(local_state)
case "$state" in
standby-running | standby-stopped) promote_standby "$state" ;;
primary-running) keep_running_primary ;;
primary-stopped) restart_stopped_primary ;;
absent) refuse no_data "there is no data directory at $PG_DATADIR" ;;
*) refuse unknown_state "local PostgreSQL is in an unknown state" ;;
esac
