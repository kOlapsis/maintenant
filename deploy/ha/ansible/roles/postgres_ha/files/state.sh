#!/bin/sh
# Prints the PostgreSQL role of this node as one JSON object, for the operator,
# lab status and lab giveback.

set -eu

# shellcheck source-path=SCRIPTDIR source=common.sh
. /usr/local/lib/maintenant-pg/common.sh

state=$(local_state)
case "$state" in
primary-running) role=primary ;;
standby-running) role=standby ;;
primary-stopped | standby-stopped) role=stopped ;;
*) role=$state ;;
esac

receiver=""
standby_state=""
standby_sync=""
standby_lag=""
if [ "$role" = standby ]; then
	receiver=$(receiver_status)
elif [ "$role" = primary ]; then
	row=$(local_sql "select state || '|' || sync_state || '|' || pg_wal_lsn_diff(pg_current_wal_flush_lsn(), flush_lsn)::bigint from pg_stat_replication where application_name = '$HA_PEER_NAME'" | head -n 1)
	standby_state=$(printf '%s' "$row" | cut -d'|' -f1)
	standby_sync=$(printf '%s' "$row" | cut -d'|' -f2)
	standby_lag=$(printf '%s' "$row" | cut -d'|' -f3)
fi

fenced_reason=""
if [ -f "$HA_FENCED" ]; then
	fenced_reason=$(cat "$HA_FENCED")
fi

jq -n \
	--arg node "$HA_SELF" \
	--arg role "$role" \
	--arg state "$state" \
	--arg fenced "$fenced_reason" \
	--argjson synced "$([ -f "$HA_SYNCED" ] && echo true || echo false)" \
	--argjson allowed "$([ -f "$HA_ALLOWANCE" ] && echo true || echo false)" \
	--argjson async "$([ -f "$HA_ASYNC_CONF" ] && echo true || echo false)" \
	--arg standby_missing "$HA_STANDBY_MISSING" \
	--arg receiver "$receiver" \
	--arg standby_state "$standby_state" \
	--arg standby_sync "$standby_sync" \
	--arg standby_lag "$standby_lag" \
	--arg package "$(dpkg-query -W --showformat='${Version}' "postgresql-$PG_VERSION" 2>/dev/null || echo "")" \
	'def nul(s): if s == "" then null else s end;
	{
		node: $node,
		role: $role,
		state: $state,
		fenced: ($fenced != ""),
		fenced_reason: nul($fenced),
		synced: $synced,
		primary_allowed: $allowed,
		standby_missing: $standby_missing,
		sync_dropped: $async,
		receiver: nul($receiver),
		standby: (if $role != "primary" then null
			elif $standby_state == "" then {state: "absent", sync_state: null, lag_bytes: null}
			else {state: $standby_state, sync_state: $standby_sync, lag_bytes: ($standby_lag | tonumber? // null)}
			end),
		package_version: nul($package)
	}'
