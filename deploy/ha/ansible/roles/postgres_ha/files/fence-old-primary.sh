#!/bin/sh
# Locks this node out of write service: stops a local primary and marks the node
# overtaken. Only a rebuild as a standby lifts the mark.
#
#   maintenant-pg-fence <reason>

set -eu

# shellcheck source-path=SCRIPTDIR source=common.sh
. /usr/local/lib/maintenant-pg/common.sh

reason="${1:-}"
if [ -z "$reason" ]; then
	echo "usage: $0 <reason>" >&2
	exit 2
fi

if [ "$(local_state)" = primary-running ]; then
	stop_local
fi
mark_fenced "$reason"
