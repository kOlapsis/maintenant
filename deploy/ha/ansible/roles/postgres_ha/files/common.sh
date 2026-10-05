# shellcheck shell=sh disable=SC2034 # a library: the scripts that source it use these
# shellcheck source=/dev/null
. /etc/maintenant-pg/ha.conf

PGPASSFILE="$HA_ETC_DIR/pgpass"
export PGPASSFILE

HA_RUN_DIR=/run/maintenant-pg
HA_ALLOWANCE="$HA_RUN_DIR/primary-allowed"
HA_ABSENT_SINCE="$HA_RUN_DIR/standby-absent-since"
HA_FENCED="$HA_STATE_DIR/fenced"
HA_SYNCED="$HA_STATE_DIR/standby-synced"
HA_EVENTS="$HA_STATE_DIR/events.log"
HA_ASYNC_CONF="$PG_CONFDIR/conf.d/zz-maintenant-async.conf"

now() {
	date -u +%Y-%m-%dT%H:%M:%SZ
}

log_event() {
	line="$(now) node=$HA_SELF event=$*"
	printf '%s\n' "$line" >>"$HA_EVENTS"
	logger -t maintenant-pg -- "$line"
	printf '%s\n' "$line" >&2
}

as_postgres() {
	runuser -u postgres -- "$@"
}

local_sql() {
	as_postgres "$PG_BINDIR/psql" -X -A -t -q -v ON_ERROR_STOP=1 \
		-h /var/run/postgresql -p "$PG_PORT" -d postgres -c "$1"
}

peer_sql() {
	as_postgres "$PG_BINDIR/psql" -X -A -t -q -v ON_ERROR_STOP=1 \
		"host=$HA_PEER_ADDRESS port=$PG_PORT user=$HA_USER dbname=postgres connect_timeout=3" -c "$1"
}

local_running() {
	local_sql 'select 1' >/dev/null 2>&1
}

# absent, primary-running, standby-running, primary-stopped, standby-stopped or unknown.
local_state() {
	if [ ! -f "$PG_DATADIR/PG_VERSION" ]; then
		echo absent
		return 0
	fi
	if local_running; then
		recovery=$(local_sql 'select pg_is_in_recovery()' 2>/dev/null) || recovery=""
		case "$recovery" in
		t) echo standby-running ;;
		f) echo primary-running ;;
		*) echo unknown ;;
		esac
	elif [ -f "$PG_DATADIR/standby.signal" ]; then
		echo standby-stopped
	else
		echo primary-stopped
	fi
}

# primary, standby or unreachable. Unreachable proves nothing about the peer.
peer_role() {
	recovery=$(peer_sql 'select pg_is_in_recovery()' 2>/dev/null) || recovery=""
	case "$recovery" in
	f) echo primary ;;
	t) echo standby ;;
	*) echo unreachable ;;
	esac
}

receiver_status() {
	local_sql 'select status from pg_stat_wal_receiver' 2>/dev/null || echo ""
}

wait_ready() {
	deadline=$(($(date +%s) + HA_READY_TIMEOUT))
	until as_postgres "$PG_BINDIR/pg_isready" -q -h /var/run/postgresql -p "$PG_PORT"; do
		if [ "$(date +%s)" -ge "$deadline" ]; then
			return 1
		fi
		sleep 1
	done
}

start_local() {
	systemctl start "$PG_UNIT" && wait_ready
}

stop_local() {
	systemctl stop "$PG_UNIT"
}

allow_primary() {
	mkdir -p "$HA_RUN_DIR"
	: >"$HA_ALLOWANCE"
}

# The data directory is marked, and given a standby.signal so that even a start
# that bypasses the unit's guard comes up read-only.
mark_fenced() {
	before=$(local_state)
	mkdir -p "$HA_STATE_DIR"
	printf 'at=%s reason=%s\n' "$(now)" "$1" >"$HA_FENCED"
	rm -f "$HA_ALLOWANCE" "$HA_SYNCED" "$HA_ASYNC_CONF" "$HA_ABSENT_SINCE"
	if [ -f "$PG_DATADIR/PG_VERSION" ]; then
		as_postgres touch "$PG_DATADIR/standby.signal"
	fi
	log_event "fenced state_before=$before reason=\"$1\""
}

drop_sync() {
	printf "synchronous_standby_names = ''\n" >"$HA_ASYNC_CONF"
	chown postgres:postgres "$HA_ASYNC_CONF"
	local_sql 'select pg_reload_conf()' >/dev/null
}

restore_sync() {
	rm -f "$HA_ASYNC_CONF"
	local_sql 'select pg_reload_conf()' >/dev/null
}

# The slot this node streams from on the peer: ok, lost, missing or unreachable.
own_slot_on_peer() {
	status=$(peer_sql "select coalesce(max(wal_status), 'missing') from pg_replication_slots where slot_name = '$HA_SELF_NAME'" 2>/dev/null) || status=unreachable
	case "$status" in
	lost | missing | unreachable) echo "$status" ;;
	*) echo ok ;;
	esac
}
