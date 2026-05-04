#!/usr/bin/env bats
# Tests for configuration flag parsing (US2)
bats_require_minimum_version 1.5.0

load 'setup'

SCRIPT="$(cd "$(dirname "$BATS_TEST_FILENAME")/.." && pwd)/install.sh"

# ── parse_maintenant_flags ────────────────────────────────────────────────────

@test "parse_maintenant_flags: separates --no-service from binary flags" {
    run bash -c "
        NO_COLOR=1 . '$SCRIPT'
        parse_maintenant_flags --no-service --addr 127.0.0.1:9000
        echo \"NO_SERVICE=\${NO_SERVICE}\"
        echo \"BINARY_FLAG_KEYS=\${BINARY_FLAG_KEYS}\"
    "
    [ "$status" -eq 0 ]
    [[ "$output" == *"NO_SERVICE=1"* ]]
    [[ "$output" == *"BINARY_FLAG_KEYS="* ]]
    [[ "$output" == *"addr"* ]]
}

@test "parse_maintenant_flags: --uninstall sets DO_UNINSTALL" {
    run bash -c "
        NO_COLOR=1 . '$SCRIPT'
        parse_maintenant_flags --uninstall
        echo \"DO_UNINSTALL=\${DO_UNINSTALL}\"
    "
    [ "$status" -eq 0 ]
    [[ "$output" == *"DO_UNINSTALL=1"* ]]
}

@test "parse_maintenant_flags: --purge sets DO_PURGE" {
    run bash -c "
        NO_COLOR=1 . '$SCRIPT'
        parse_maintenant_flags --uninstall --purge
        echo \"DO_PURGE=\${DO_PURGE}\"
    "
    [ "$status" -eq 0 ]
    [[ "$output" == *"DO_PURGE=1"* ]]
}

@test "parse_maintenant_flags: boolean flags stored as true" {
    run bash -c "
        NO_COLOR=1 . '$SCRIPT'
        parse_maintenant_flags --disableTelemetry --mcp
        printf '%s\n' \"\$BINARY_FLAG_KEYS\"
    "
    [ "$status" -eq 0 ]
    [[ "$output" == *"disableTelemetry"* ]]
    [[ "$output" == *"mcp"* ]]
}

@test "parse_maintenant_flags: unknown argument exits 2" {
    run bash -c "
        NO_COLOR=1 . '$SCRIPT'
        parse_maintenant_flags --unknownflag
    "
    [ "$status" -eq 2 ]
}

# ── flag_to_env ───────────────────────────────────────────────────────────────

@test "flag_to_env: addr -> MAINTENANT_ADDR" {
    result=$(bash -c "NO_COLOR=1 . '$SCRIPT'; flag_to_env addr")
    [ "$result" = "MAINTENANT_ADDR" ]
}

@test "flag_to_env: baseUrl -> MAINTENANT_BASE_URL" {
    result=$(bash -c "NO_COLOR=1 . '$SCRIPT'; flag_to_env baseUrl")
    [ "$result" = "MAINTENANT_BASE_URL" ]
}

@test "flag_to_env: organisationName -> MAINTENANT_ORGANISATION_NAME" {
    result=$(bash -c "NO_COLOR=1 . '$SCRIPT'; flag_to_env organisationName")
    [ "$result" = "MAINTENANT_ORGANISATION_NAME" ]
}

@test "flag_to_env: licenseKey -> MAINTENANT_LICENSE_KEY" {
    result=$(bash -c "NO_COLOR=1 . '$SCRIPT'; flag_to_env licenseKey")
    [ "$result" = "MAINTENANT_LICENSE_KEY" ]
}

@test "flag_to_env: smtpPassword -> MAINTENANT_SMTP_PASSWORD" {
    result=$(bash -c "NO_COLOR=1 . '$SCRIPT'; flag_to_env smtpPassword")
    [ "$result" = "MAINTENANT_SMTP_PASSWORD" ]
}

@test "flag_to_env: mcpClientSecret -> MAINTENANT_MCP_CLIENT_SECRET" {
    result=$(bash -c "NO_COLOR=1 . '$SCRIPT'; flag_to_env mcpClientSecret")
    [ "$result" = "MAINTENANT_MCP_CLIENT_SECRET" ]
}

@test "flag_to_env: k8sNamespaces -> MAINTENANT_K8S_NAMESPACES" {
    result=$(bash -c "NO_COLOR=1 . '$SCRIPT'; flag_to_env k8sNamespaces")
    [ "$result" = "MAINTENANT_K8S_NAMESPACES" ]
}

@test "flag_to_env: k8sExcludeNamespaces -> MAINTENANT_K8S_EXCLUDE_NAMESPACES" {
    result=$(bash -c "NO_COLOR=1 . '$SCRIPT'; flag_to_env k8sExcludeNamespaces")
    [ "$result" = "MAINTENANT_K8S_EXCLUDE_NAMESPACES" ]
}

@test "flag_to_env: updateInterval -> MAINTENANT_UPDATE_INTERVAL" {
    result=$(bash -c "NO_COLOR=1 . '$SCRIPT'; flag_to_env updateInterval")
    [ "$result" = "MAINTENANT_UPDATE_INTERVAL" ]
}

@test "flag_to_env: securityScoreThreshold -> MAINTENANT_SECURITY_SCORE_THRESHOLD" {
    result=$(bash -c "NO_COLOR=1 . '$SCRIPT'; flag_to_env securityScoreThreshold")
    [ "$result" = "MAINTENANT_SECURITY_SCORE_THRESHOLD" ]
}

@test "flag_to_env: disableTelemetry -> MAINTENANT_DISABLE_TELEMETRY" {
    result=$(bash -c "NO_COLOR=1 . '$SCRIPT'; flag_to_env disableTelemetry")
    [ "$result" = "MAINTENANT_DISABLE_TELEMETRY" ]
}

@test "flag_to_env: allowPrivateWebhooks -> MAINTENANT_ALLOW_PRIVATE_WEBHOOKS" {
    result=$(bash -c "NO_COLOR=1 . '$SCRIPT'; flag_to_env allowPrivateWebhooks")
    [ "$result" = "MAINTENANT_ALLOW_PRIVATE_WEBHOOKS" ]
}

@test "flag_to_env: corsOrigins -> MAINTENANT_CORS_ORIGINS" {
    result=$(bash -c "NO_COLOR=1 . '$SCRIPT'; flag_to_env corsOrigins")
    [ "$result" = "MAINTENANT_CORS_ORIGINS" ]
}

@test "flag_to_env: maxBodySize -> MAINTENANT_MAX_BODY_SIZE" {
    result=$(bash -c "NO_COLOR=1 . '$SCRIPT'; flag_to_env maxBodySize")
    [ "$result" = "MAINTENANT_MAX_BODY_SIZE" ]
}

@test "flag_to_env: logLevel -> MAINTENANT_LOG_LEVEL" {
    result=$(bash -c "NO_COLOR=1 . '$SCRIPT'; flag_to_env logLevel")
    [ "$result" = "MAINTENANT_LOG_LEVEL" ]
}

@test "flag_to_env: runtime -> MAINTENANT_RUNTIME" {
    result=$(bash -c "NO_COLOR=1 . '$SCRIPT'; flag_to_env runtime")
    [ "$result" = "MAINTENANT_RUNTIME" ]
}

# ── merge_env_file ────────────────────────────────────────────────────────────

@test "merge_env_file: creates file if not exists" {
    FAKE_TMPDIR=$(mktemp -d)
    FAKE_CONFIG_DIR=$(mktemp -d)

    run bash -c "
        NO_COLOR=1
        TMPDIR_INSTALL='$FAKE_TMPDIR'
        CONFIG_DIR='$FAKE_CONFIG_DIR'
        SERVICE_USER=nobody
        chown() { return 0; }
        export -f chown
        export NO_COLOR TMPDIR_INSTALL CONFIG_DIR SERVICE_USER
        . '$SCRIPT'
        BINARY_FLAG_KEYS='addr
'
        BINARY_FLAG_VALS='127.0.0.1:9000
'
        export BINARY_FLAG_KEYS BINARY_FLAG_VALS
        merge_env_file
    "
    [ "$status" -eq 0 ]
    [ -f "$FAKE_CONFIG_DIR/maintenant.env" ]
    grep -q "MAINTENANT_ADDR=127.0.0.1:9000" "$FAKE_CONFIG_DIR/maintenant.env"
    rm -rf "$FAKE_TMPDIR" "$FAKE_CONFIG_DIR"
}

@test "merge_env_file: preserves existing keys not being updated" {
    FAKE_TMPDIR=$(mktemp -d)
    FAKE_CONFIG_DIR=$(mktemp -d)
    # Pre-existing env file with two keys
    printf 'MAINTENANT_ADDR=127.0.0.1:8080\nMAINTENANT_DB=./old.db\n' \
        > "$FAKE_CONFIG_DIR/maintenant.env"

    run bash -c "
        NO_COLOR=1
        TMPDIR_INSTALL='$FAKE_TMPDIR'
        CONFIG_DIR='$FAKE_CONFIG_DIR'
        SERVICE_USER=nobody
        chown() { return 0; }
        export -f chown
        export NO_COLOR TMPDIR_INSTALL CONFIG_DIR SERVICE_USER
        . '$SCRIPT'
        BINARY_FLAG_KEYS='addr
'
        BINARY_FLAG_VALS='0.0.0.0:9000
'
        export BINARY_FLAG_KEYS BINARY_FLAG_VALS
        merge_env_file
    "
    [ "$status" -eq 0 ]
    # addr updated
    grep -q "MAINTENANT_ADDR=0.0.0.0:9000" "$FAKE_CONFIG_DIR/maintenant.env"
    # db preserved
    grep -q "MAINTENANT_DB=./old.db" "$FAKE_CONFIG_DIR/maintenant.env"
    rm -rf "$FAKE_TMPDIR" "$FAKE_CONFIG_DIR"
}

@test "merge_env_file: resulting file has mode 0640" {
    FAKE_TMPDIR=$(mktemp -d)
    FAKE_CONFIG_DIR=$(mktemp -d)

    bash -c "
        NO_COLOR=1
        TMPDIR_INSTALL='$FAKE_TMPDIR'
        CONFIG_DIR='$FAKE_CONFIG_DIR'
        SERVICE_USER=nobody
        export NO_COLOR TMPDIR_INSTALL CONFIG_DIR SERVICE_USER
        . '$SCRIPT'
        BINARY_FLAG_KEYS='addr
'
        BINARY_FLAG_VALS='127.0.0.1:8080
'
        export BINARY_FLAG_KEYS BINARY_FLAG_VALS
        merge_env_file
    "
    PERMS=$(stat -c '%a' "$FAKE_CONFIG_DIR/maintenant.env")
    [ "$PERMS" = "640" ]
    rm -rf "$FAKE_TMPDIR" "$FAKE_CONFIG_DIR"
}
