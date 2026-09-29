#!/bin/sh
# The app#pg resource: start, stop and check the primary role on this node.
#
#   start  start the primary the promotion allowed, then prove it is the only one
#   stop   withdraw the allowance and stop a primary; a standby is left running
#   check  0 when this node runs the allowed primary, 1 otherwise

set -eu

# shellcheck source-path=SCRIPTDIR source=common.sh
. /usr/local/lib/maintenant-pg/common.sh

start() {
	state=$(local_state)
	if [ "$state" = primary-stopped ]; then
		if [ ! -f "$HA_ALLOWANCE" ]; then
			echo "the primary is not allowed to start here" >&2
			exit 1
		fi
		if ! start_local; then
			log_event "primary_start_failed"
			exit 1
		fi
		log_event "primary_started"
	elif [ "$state" != primary-running ]; then
		echo "local PostgreSQL is $state, not a primary: the promotion did not happen" >&2
		exit 1
	fi

	if [ "$(local_sql 'select pg_is_in_recovery()')" != f ]; then
		echo "local PostgreSQL is still in recovery" >&2
		exit 1
	fi
	if [ "$(peer_role)" = primary ]; then
		stop_local
		log_event "two_primaries action=stopped_local"
		exit 1
	fi
}

stop() {
	rm -f "$HA_ALLOWANCE"
	state=$(local_state)
	if [ "$state" = primary-running ]; then
		stop_local
		log_event "primary_stopped"
	else
		echo "local PostgreSQL is $state: only a primary stops with the service" >&2
	fi
}

check() {
	[ -f "$HA_ALLOWANCE" ] && [ "$(local_state)" = primary-running ]
}

case "${1:-}" in
start) start ;;
stop) stop ;;
check) check ;;
*)
	echo "usage: $0 start|stop|check" >&2
	exit 2
	;;
esac
