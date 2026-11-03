#!/usr/bin/env bats

LAB="$BATS_TEST_DIRNAME/../lab"
VERDICT="$BATS_TEST_DIRNAME/../verdict.jq"
FIXTURES="$BATS_TEST_DIRNAME/fixtures"

@test "lab parses as sh" {
	sh -n "$LAB"
}

@test "lab without a command exits 2" {
	run "$LAB"
	[ "$status" -eq 2 ]
}

@test "lab with an unknown command exits 2" {
	run "$LAB" bogus
	[ "$status" -eq 2 ]
}

@test "a complete three-level run passes the verdict" {
	jq -f "$VERDICT" "$FIXTURES/report-pass.json" | jq -e '.pass == true and (.failures | length == 0)'
}

@test "a run stopped by acknowledgment fails the verdict and names what is missing" {
	out=$(jq -f "$VERDICT" "$FIXTURES/report-fail.json")
	echo "$out" | jq -e '.pass == false'
	echo "$out" | jq -e '.failures | index("run ended stopped_by_ack, not exhausted") != null'
	echo "$out" | jq -e '.failures | index("level 3 not sent to the CTO email") != null'
	echo "$out" | jq -e '.failures | index("cto@lab.test received 1 mail(s), expected at least 2") != null'
}
