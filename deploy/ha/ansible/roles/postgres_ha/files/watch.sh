#!/bin/sh
# Runs on both nodes for as long as they are up. On a node that was overtaken it
# rebuilds the standby; on a standby it watches its slot and records the first
# full catch-up; on the primary it applies the configured conduct when the
# standby goes missing, and restores synchronous replication once it is less than a WAL segment behind.

set -eu

# shellcheck source-path=SCRIPTDIR source=common.sh
. /usr/local/lib/maintenant-pg/common.sh

REBUILD="$HA_SBIN_DIR/maintenant-pg-rebuild"
FENCE="$HA_SBIN_DIR/maintenant-pg-fence"

# The peer's replication row for the standby: "<state> <sync_state> <lag bytes>".
standby_row() {
	query="select state || ' ' || sync_state || ' ' || pg_wal_lsn_diff(pg_current_wal_flush_lsn(), flush_lsn)::bigint from pg_stat_replication where application_name = '$1'"
	if [ "$2" = local ]; then
		local_sql "$query" 2>/dev/null | head -n 1
	else
		peer_sql "$query" 2>/dev/null | head -n 1
	fi
}

watch_primary() {
	# shellcheck disable=SC2046 # the row is three words on purpose
	set -- $(standby_row "$HA_PEER_NAME" local)
	streaming=no
	near=no
	if [ "${1:-}" = streaming ]; then
		streaming=yes
		segment=$(local_sql "select pg_size_bytes(current_setting('wal_segment_size'))")
		if [ "${3:-$segment}" -lt "$segment" ]; then
			near=yes
		fi
	fi

	if [ "$streaming" = yes ] && [ -f "$HA_ABSENT_SINCE" ]; then
		rm -f "$HA_ABSENT_SINCE"
		log_event "standby_back"
	elif [ "$streaming" = no ] && [ ! -f "$HA_ABSENT_SINCE" ]; then
		mkdir -p "$HA_RUN_DIR"
		date +%s >"$HA_ABSENT_SINCE"
		log_event "standby_absent standby_missing=$HA_STANDBY_MISSING"
	fi

	if [ -f "$HA_ASYNC_CONF" ]; then
		if [ "$near" = yes ]; then
			restore_sync
			log_event "sync_restored loss_window=closed"
		fi
	elif [ "$HA_STANDBY_MISSING" = continue ] && [ -f "$HA_ABSENT_SINCE" ]; then
		since=$(cat "$HA_ABSENT_SINCE")
		if [ $(($(date +%s) - since)) -ge "$HA_STANDBY_MISSING_GRACE" ]; then
			drop_sync
			log_event "sync_dropped loss_window=open reason=standby_absent"
		fi
	fi
}

watch_standby() {
	slot=$(own_slot_on_peer)
	if [ "$slot" = lost ] || [ "$slot" = missing ]; then
		log_event "retention_exceeded slot=$slot action=full_rebuild"
		"$REBUILD" --full || log_event "rebuild_failed method=basebackup"
		return 0
	fi
	if [ -f "$HA_SYNCED" ]; then
		return 0
	fi
	# shellcheck disable=SC2046 # the row is three words on purpose
	set -- $(standby_row "$HA_SELF_NAME" peer)
	if [ "${1:-}" = streaming ] && { [ "${2:-}" = sync ] || [ "${3:-}" = 0 ]; }; then
		: >"$HA_SYNCED"
		log_event "standby_synced sync_state=${2:-}"
	fi
}

tick() {
	if [ -f "$HA_FENCED" ]; then
		if [ "$(peer_role)" = primary ]; then
			"$REBUILD" || log_event "rebuild_failed"
		fi
		return 0
	fi

	state=$(local_state)
	case "$state" in
	primary-running | primary-stopped)
		if [ ! -f "$HA_ALLOWANCE" ]; then
			if [ "$(peer_role)" = primary ]; then
				"$FENCE" "former primary while $HA_PEER is primary"
			fi
		elif [ "$state" = primary-running ]; then
			watch_primary
		fi
		;;
	standby-running) watch_standby ;;
	standby-stopped) start_local || log_event "standby_start_failed" ;;
	esac
}

if [ "${1:-}" = --once ]; then
	tick
	exit 0
fi

# Each pass is its own process: set -e does not apply inside a function whose status is tested.
while :; do
	if ! "$0" --once; then
		echo "watch: the last pass failed, retrying in ${HA_WATCH_INTERVAL}s" >&2
	fi
	sleep "$HA_WATCH_INTERVAL"
done
