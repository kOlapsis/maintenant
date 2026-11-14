#!/usr/bin/env bats

SCENARIOS="$BATS_TEST_DIRNAME/../scenarios"
MODES='["ip-only", "postgres/streaming", "sqlite/drbd", "postgres/drbd"]'

scenario() {
	python3 -c 'import json,sys,yaml; json.dump(yaml.safe_load(open(sys.argv[1])), sys.stdout)' "$1"
}

@test "every scenario file has its id, an action per step and an expectation per mode" {
	for file in "$SCENARIOS"/[0-9][0-9]-*.yml; do
		base=$(basename "$file" .yml)
		scenario "$file" | jq -e --argjson id "$((10#${base%%-*}))" --arg slug "${base#*-}" --argjson modes "$MODES" '
			.id == $id and .slug == $slug and (.name | length > 0) and (.repetitions >= 1)
			and all(.injection, .repair, .verify;
				(.on | IN("host", "active", "passive", "arbitrator", "measurement")) and (.command | length > 0))
			and (.measures | length > 0)
			and ((.modes | keys) == ($modes | sort))
			and all(.modes[]; if .applicable then (.expected | length > 0) else (.reason | length > 0) end)
		' >/dev/null || {
			echo "invalid scenario: $file" >&2
			return 1
		}
	done
}

@test "every scenario command parses as sh" {
	for file in "$SCENARIOS"/[0-9][0-9]-*.yml; do
		for step in injection repair verify; do
			scenario "$file" | jq -r --arg s "$step" '.[$s].command' | sh -n || {
				echo "$file: $step does not parse" >&2
				return 1
			}
		done
	done
}

@test "the DRBD scenarios take their reserved numbers and apply to sqlite/drbd only among the built modes" {
	for n in 20 21 22; do
		file=$(ls "$SCENARIOS/$n"-*.yml)
		scenario "$file" | jq -e '.modes["sqlite/drbd"].applicable
			and (.modes["ip-only"].applicable | not) and (.modes["postgres/streaming"].applicable | not)' >/dev/null
	done
}

@test "every expected cluster alert names an event the alert script sends" {
	known=$(grep -o 'event: "[a-z_]*"' "$BATS_TEST_DIRNAME/../../ansible/roles/cluster_alert/files/maintenant-cluster-alert" |
		cut -d'"' -f2 | jq -Rsc 'split("\n") | map(select(length > 0))')
	for file in "$SCENARIOS"/[0-9][0-9]-*.yml; do
		scenario "$file" | jq -e --argjson known "$known" '
			[(.cluster_alerts // []), (.modes[].cluster_alerts // [])] | add
			| all(.[]; type == "object" and (.event | IN($known[])))
		' >/dev/null || {
			echo "unknown cluster alert in $file" >&2
			return 1
		}
	done
}
