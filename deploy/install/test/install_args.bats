#!/usr/bin/env bats
# Tests for configuration flag parsing (US2)
bats_require_minimum_version 1.5.0

load 'setup'

SCRIPT="$(cd "$(dirname "$BATS_TEST_FILENAME")/.." && pwd)/install.sh"

# ── parse_maintenant_flags ────────────────────────────────────────────────────

@test "parse_maintenant_flags: separates --no-service from binary flags" {
    run bash -c "
        _INSTALL_SH_TESTING=1 NO_COLOR=1 . '$SCRIPT'
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
        _INSTALL_SH_TESTING=1 NO_COLOR=1 . '$SCRIPT'
        parse_maintenant_flags --uninstall
        echo \"DO_UNINSTALL=\${DO_UNINSTALL}\"
    "
    [ "$status" -eq 0 ]
    [[ "$output" == *"DO_UNINSTALL=1"* ]]
}

@test "parse_maintenant_flags: --purge sets DO_PURGE" {
    run bash -c "
        _INSTALL_SH_TESTING=1 NO_COLOR=1 . '$SCRIPT'
        parse_maintenant_flags --uninstall --purge
        echo \"DO_PURGE=\${DO_PURGE}\"
    "
    [ "$status" -eq 0 ]
    [[ "$output" == *"DO_PURGE=1"* ]]
}

@test "parse_maintenant_flags: boolean flags stored as true" {
    run bash -c "
        _INSTALL_SH_TESTING=1 NO_COLOR=1 . '$SCRIPT'
        parse_maintenant_flags --disableTelemetry --mcp
        printf '%s\n' \"\$BINARY_FLAG_KEYS\"
    "
    [ "$status" -eq 0 ]
    [[ "$output" == *"disableTelemetry"* ]]
    [[ "$output" == *"mcp"* ]]
}

@test "parse_maintenant_flags: unknown argument exits 2" {
    run bash -c "
        _INSTALL_SH_TESTING=1 NO_COLOR=1 . '$SCRIPT'
        parse_maintenant_flags --unknownflag
    "
    [ "$status" -eq 2 ]
}

# ── flag_to_env ───────────────────────────────────────────────────────────────

@test "flag_to_env: addr -> MAINTENANT_ADDR" {
    result=$(bash -c "_INSTALL_SH_TESTING=1 NO_COLOR=1 . '$SCRIPT'; flag_to_env addr")
    [ "$result" = "MAINTENANT_ADDR" ]
}

@test "flag_to_env: baseUrl -> MAINTENANT_BASE_URL" {
    result=$(bash -c "_INSTALL_SH_TESTING=1 NO_COLOR=1 . '$SCRIPT'; flag_to_env baseUrl")
    [ "$result" = "MAINTENANT_BASE_URL" ]
}

@test "flag_to_env: organisationName -> MAINTENANT_ORGANISATION_NAME" {
    result=$(bash -c "_INSTALL_SH_TESTING=1 NO_COLOR=1 . '$SCRIPT'; flag_to_env organisationName")
    [ "$result" = "MAINTENANT_ORGANISATION_NAME" ]
}

@test "flag_to_env: licenseKey -> MAINTENANT_LICENSE_KEY" {
    result=$(bash -c "_INSTALL_SH_TESTING=1 NO_COLOR=1 . '$SCRIPT'; flag_to_env licenseKey")
    [ "$result" = "MAINTENANT_LICENSE_KEY" ]
}

@test "flag_to_env: smtpPassword -> MAINTENANT_SMTP_PASSWORD" {
    result=$(bash -c "_INSTALL_SH_TESTING=1 NO_COLOR=1 . '$SCRIPT'; flag_to_env smtpPassword")
    [ "$result" = "MAINTENANT_SMTP_PASSWORD" ]
}

@test "flag_to_env: mcpClientSecret -> MAINTENANT_MCP_CLIENT_SECRET" {
    result=$(bash -c "_INSTALL_SH_TESTING=1 NO_COLOR=1 . '$SCRIPT'; flag_to_env mcpClientSecret")
    [ "$result" = "MAINTENANT_MCP_CLIENT_SECRET" ]
}

@test "flag_to_env: k8sNamespaces -> MAINTENANT_K8S_NAMESPACES" {
    result=$(bash -c "_INSTALL_SH_TESTING=1 NO_COLOR=1 . '$SCRIPT'; flag_to_env k8sNamespaces")
    [ "$result" = "MAINTENANT_K8S_NAMESPACES" ]
}

@test "flag_to_env: k8sExcludeNamespaces -> MAINTENANT_K8S_EXCLUDE_NAMESPACES" {
    result=$(bash -c "_INSTALL_SH_TESTING=1 NO_COLOR=1 . '$SCRIPT'; flag_to_env k8sExcludeNamespaces")
    [ "$result" = "MAINTENANT_K8S_EXCLUDE_NAMESPACES" ]
}

@test "flag_to_env: updateInterval -> MAINTENANT_UPDATE_INTERVAL" {
    result=$(bash -c "_INSTALL_SH_TESTING=1 NO_COLOR=1 . '$SCRIPT'; flag_to_env updateInterval")
    [ "$result" = "MAINTENANT_UPDATE_INTERVAL" ]
}

@test "flag_to_env: securityScoreThreshold -> MAINTENANT_SECURITY_SCORE_THRESHOLD" {
    result=$(bash -c "_INSTALL_SH_TESTING=1 NO_COLOR=1 . '$SCRIPT'; flag_to_env securityScoreThreshold")
    [ "$result" = "MAINTENANT_SECURITY_SCORE_THRESHOLD" ]
}

@test "flag_to_env: disableTelemetry -> MAINTENANT_DISABLE_TELEMETRY" {
    result=$(bash -c "_INSTALL_SH_TESTING=1 NO_COLOR=1 . '$SCRIPT'; flag_to_env disableTelemetry")
    [ "$result" = "MAINTENANT_DISABLE_TELEMETRY" ]
}

@test "flag_to_env: allowPrivateWebhooks -> MAINTENANT_ALLOW_PRIVATE_WEBHOOKS" {
    result=$(bash -c "_INSTALL_SH_TESTING=1 NO_COLOR=1 . '$SCRIPT'; flag_to_env allowPrivateWebhooks")
    [ "$result" = "MAINTENANT_ALLOW_PRIVATE_WEBHOOKS" ]
}

@test "flag_to_env: corsOrigins -> MAINTENANT_CORS_ORIGINS" {
    result=$(bash -c "_INSTALL_SH_TESTING=1 NO_COLOR=1 . '$SCRIPT'; flag_to_env corsOrigins")
    [ "$result" = "MAINTENANT_CORS_ORIGINS" ]
}

@test "flag_to_env: maxBodySize -> MAINTENANT_MAX_BODY_SIZE" {
    result=$(bash -c "_INSTALL_SH_TESTING=1 NO_COLOR=1 . '$SCRIPT'; flag_to_env maxBodySize")
    [ "$result" = "MAINTENANT_MAX_BODY_SIZE" ]
}

@test "flag_to_env: logLevel -> MAINTENANT_LOG_LEVEL" {
    result=$(bash -c "_INSTALL_SH_TESTING=1 NO_COLOR=1 . '$SCRIPT'; flag_to_env logLevel")
    [ "$result" = "MAINTENANT_LOG_LEVEL" ]
}

@test "flag_to_env: runtime -> MAINTENANT_RUNTIME" {
    result=$(bash -c "_INSTALL_SH_TESTING=1 NO_COLOR=1 . '$SCRIPT'; flag_to_env runtime")
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
        _INSTALL_SH_TESTING=1 . '$SCRIPT'
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
        _INSTALL_SH_TESTING=1 . '$SCRIPT'
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
        chown() { return 0; }
        export -f chown
        export NO_COLOR TMPDIR_INSTALL CONFIG_DIR SERVICE_USER
        _INSTALL_SH_TESTING=1 . '$SCRIPT'
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

@test "merge_env_file: keeps every line it was not asked to change" {
    FAKE_TMPDIR=$(mktemp -d)
    FAKE_CONFIG_DIR=$(mktemp -d)
    cat > "$FAKE_CONFIG_DIR/maintenant.env" <<'ENV'
# Generated by install.sh
# Last updated: 2020-01-01T00:00:00Z
# Script version: abc1234
# Edit this file directly then: systemctl restart maintenant

MAINTENANT_ADDR=127.0.0.1:8080
# socket proxy in front of the Docker API
DOCKER_HOST=tcp://socket-proxy:2375
MAINTENANT_K8S_NAMESPACES=prod,staging
ENV

    run bash -c "
        NO_COLOR=1
        TMPDIR_INSTALL='$FAKE_TMPDIR'
        CONFIG_DIR='$FAKE_CONFIG_DIR'
        SERVICE_USER=nobody
        chown() { return 0; }
        export -f chown
        export NO_COLOR TMPDIR_INSTALL CONFIG_DIR SERVICE_USER
        _INSTALL_SH_TESTING=1 . '$SCRIPT'
        parse_maintenant_flags --addr 0.0.0.0:9000 --logLevel debug
        merge_env_file
    "
    [ "$status" -eq 0 ]
    ENV_FILE="$FAKE_CONFIG_DIR/maintenant.env"
    grep -qx 'MAINTENANT_ADDR=0.0.0.0:9000' "$ENV_FILE"
    grep -qx 'MAINTENANT_LOG_LEVEL=debug' "$ENV_FILE"
    grep -qx 'DOCKER_HOST=tcp://socket-proxy:2375' "$ENV_FILE"
    grep -qx 'MAINTENANT_K8S_NAMESPACES=prod,staging' "$ENV_FILE"
    grep -qx '# socket proxy in front of the Docker API' "$ENV_FILE"
    [ "$(grep -c '^MAINTENANT_ADDR=' "$ENV_FILE")" -eq 1 ]
    [ "$(grep -c '^# Generated by install.sh$' "$ENV_FILE")" -eq 1 ]
    [ -z "$(grep '2020-01-01' "$ENV_FILE")" ]
    rm -rf "$FAKE_TMPDIR" "$FAKE_CONFIG_DIR"
}

@test "merge_env_file: a flag given twice keeps its last value" {
    FAKE_TMPDIR=$(mktemp -d)
    FAKE_CONFIG_DIR=$(mktemp -d)
    printf 'MAINTENANT_ADDR=127.0.0.1:8080\n' > "$FAKE_CONFIG_DIR/maintenant.env"

    run bash -c "
        NO_COLOR=1
        TMPDIR_INSTALL='$FAKE_TMPDIR'
        CONFIG_DIR='$FAKE_CONFIG_DIR'
        SERVICE_USER=nobody
        chown() { return 0; }
        export -f chown
        export NO_COLOR TMPDIR_INSTALL CONFIG_DIR SERVICE_USER
        _INSTALL_SH_TESTING=1 . '$SCRIPT'
        parse_maintenant_flags --addr 0.0.0.0:1 --addr 0.0.0.0:2
        merge_env_file
    "
    [ "$status" -eq 0 ]
    [ "$(grep '^MAINTENANT_ADDR=' "$FAKE_CONFIG_DIR/maintenant.env")" = "MAINTENANT_ADDR=0.0.0.0:2" ]
    rm -rf "$FAKE_TMPDIR" "$FAKE_CONFIG_DIR"
}

# ── boolean and value flags ───────────────────────────────────────────────────

parse_and_print() {
    bash -c "
        _INSTALL_SH_TESTING=1 NO_COLOR=1 . '$SCRIPT'
        parse_maintenant_flags $*
        printf '%s' \"\$BINARY_FLAG_KEYS\" | while IFS= read -r k; do
            printf '%s=%s\n' \"\$k\" \"\$(_binary_flag_value \"\$k\")\"
        done
    "
}

@test "parse_maintenant_flags: a boolean flag does not swallow the next flag" {
    run parse_and_print --disableOsEolRefresh --addr 0.0.0.0:8080
    [ "$status" -eq 0 ]
    [[ "$output" == *"disableOsEolRefresh=true"* ]]
    [[ "$output" == *"addr=0.0.0.0:8080"* ]]
}

@test "parse_maintenant_flags: a boolean flag may come last" {
    run parse_and_print --addr 0.0.0.0:8080 --proxyLabels
    [ "$status" -eq 0 ]
    [[ "$output" == *"proxyLabels=true"* ]]
}

@test "parse_maintenant_flags: every boolean flag of the binary works bare" {
    run bash -c "
        _INSTALL_SH_TESTING=1 NO_COLOR=1 . '$SCRIPT'
        for f in \$BOOL_FLAGS; do
            parse_maintenant_flags --\$f --addr 0.0.0.0:8080 || exit 1
            [ \"\$(_binary_flag_value \$f)\" = true ] || { echo \"--\$f\"; exit 1; }
            [ \"\$(_binary_flag_value addr)\" = 0.0.0.0:8080 ] || { echo \"--\$f\"; exit 1; }
        done
    "
    [ "$status" -eq 0 ]
}

@test "parse_maintenant_flags: a boolean flag takes =false" {
    run parse_and_print --disableTelemetry=false --mcp=TRUE
    [ "$status" -eq 0 ]
    [[ "$output" == *"disableTelemetry=false"* ]]
    [[ "$output" == *"mcp=true"* ]]
}

@test "parse_maintenant_flags: a boolean flag refuses a value that is not a boolean" {
    run parse_and_print --mcp=maybe
    [ "$status" -eq 2 ]
}

@test "parse_maintenant_flags: a value flag takes --flag=value" {
    run parse_and_print --addr=0.0.0.0:9000 --smtpPassword=--starts-with-dashes
    [ "$status" -eq 0 ]
    [[ "$output" == *"addr=0.0.0.0:9000"* ]]
    [[ "$output" == *"smtpPassword=--starts-with-dashes"* ]]
}

@test "parse_maintenant_flags: a value flag refuses the next flag as its value" {
    run parse_and_print --addr --mcp
    [ "$status" -eq 2 ]
}

@test "parse_maintenant_flags: a flag the binary does not know is refused" {
    run parse_and_print --adr 0.0.0.0:8080
    [ "$status" -eq 2 ]
    [[ "$output" == *"Unknown argument: --adr"* ]]
}

@test "parse_maintenant_flags: the binary's action flags are refused" {
    run parse_and_print --copy-store-to postgres://db/maintenant
    [ "$status" -eq 2 ]
    run parse_and_print --yes
    [ "$status" -eq 2 ]
}

@test "parse_maintenant_flags: a value on two lines is refused" {
    run bash -c "
        _INSTALL_SH_TESTING=1 NO_COLOR=1 . '$SCRIPT'
        parse_maintenant_flags --label \"\$(printf 'web\nMAINTENANT_MODE=server')\"
    "
    [ "$status" -eq 2 ]
}

@test "parse_maintenant_flags: flags the help used to omit are accepted" {
    run parse_and_print --proxyLabels --disableOsEolRefresh --nodeName node-1 \
        --trustedProxies 10.0.0.0/8 --containerDownAfter 5m \
        --agentSpoolMaxMemoryBytes 1048576 --agentSpoolMaxDiskBytes 10485760 \
        --agentSpoolMaxAgeSeconds 3600
    [ "$status" -eq 0 ]
    [[ "$output" == *"nodeName=node-1"* ]]
    [[ "$output" == *"agentSpoolMaxAgeSeconds=3600"* ]]
}

@test "--help lists the flags it used to omit" {
    run bash "$SCRIPT" --help
    [ "$status" -eq 0 ]
    for f in proxyLabels disableOsEolRefresh nodeName trustedProxies containerDownAfter \
             agentSpoolMaxMemoryBytes agentSpoolMaxDiskBytes agentSpoolMaxAgeSeconds binary sha256sums; do
        [[ "$output" == *"--$f"* ]] || { echo "missing --$f"; return 1; }
    done
}

@test "parse_maintenant_flags: --binary and --sha256sums are script flags" {
    run bash -c "
        _INSTALL_SH_TESTING=1 NO_COLOR=1 . '$SCRIPT'
        parse_maintenant_flags --binary ./maintenant --sha256sums=./SHA256SUMS
        echo \"LOCAL_BINARY=\$LOCAL_BINARY LOCAL_SUMS=\$LOCAL_SUMS KEYS=[\$BINARY_FLAG_KEYS]\"
    "
    [ "$status" -eq 0 ]
    [[ "$output" == *"LOCAL_BINARY=./maintenant LOCAL_SUMS=./SHA256SUMS KEYS=[]"* ]]
}

@test "parse_maintenant_flags: --sha256sums without --binary exits 2" {
    run parse_and_print --sha256sums ./SHA256SUMS
    [ "$status" -eq 2 ]
}
