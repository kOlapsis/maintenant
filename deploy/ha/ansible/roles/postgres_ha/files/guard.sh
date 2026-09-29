#!/bin/sh
# Runs before every start of the PostgreSQL unit. A standby may always start; a
# primary only with the allowance the promotion grants, which lives under /run
# and so never survives a reboot.

set -eu

# shellcheck source-path=SCRIPTDIR source=common.sh
. /usr/local/lib/maintenant-pg/common.sh

if [ -f "$HA_FENCED" ]; then
	log_event "start_refused reason=fenced"
	exit 1
fi

if [ -f "$PG_DATADIR/standby.signal" ] || [ -f "$HA_ALLOWANCE" ]; then
	exit 0
fi

if [ "$(peer_role)" = primary ]; then
	"$HA_SBIN_DIR/maintenant-pg-fence" "start attempted while $HA_PEER is primary"
fi
log_event "start_refused reason=primary_without_allowance"
exit 1
