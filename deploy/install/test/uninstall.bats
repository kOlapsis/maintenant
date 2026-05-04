#!/usr/bin/env bats
# Tests for uninstall / purge (US3)
bats_require_minimum_version 1.5.0

load 'setup'

SCRIPT="$(cd "$(dirname "$BATS_TEST_FILENAME")/.." && pwd)/install.sh"

# Helper: run uninstall in a sandboxed environment
run_uninstall() {
    FAKE_INSTALL_DIR="$1"
    FAKE_DATA_DIR="$2"
    FAKE_CONFIG_DIR="$3"
    shift 3
    bash -c "
        NO_COLOR=1
        id()        { echo 0; }
        systemctl() { return 0; }
        getent()    { return 1; }
        userdel()   { return 0; }
        gpasswd()   { return 0; }
        export -f id systemctl getent userdel gpasswd
        INSTALL_DIR='$FAKE_INSTALL_DIR'
        DATA_DIR='$FAKE_DATA_DIR'
        CONFIG_DIR='$FAKE_CONFIG_DIR'
        export INSTALL_DIR DATA_DIR CONFIG_DIR NO_COLOR
        _INSTALL_SH_TESTING=1 . '$SCRIPT'
        parse_maintenant_flags $(printf '%s ' "$@")
        check_prereqs
        uninstall
    "
}

@test "uninstall: removes binary but preserves data dir" {
    FAKE_INSTALL_DIR=$(mktemp -d)
    FAKE_DATA_DIR=$(mktemp -d)
    FAKE_CONFIG_DIR=$(mktemp -d)

    # Simulate installed binary and service file
    touch "$FAKE_INSTALL_DIR/maintenant"
    SERVICE_FILE="/tmp/maintenant-test-$$.service"
    touch "$SERVICE_FILE"

    run bash -c "
        NO_COLOR=1
        id()        { echo 0; }
        systemctl() { return 0; }
        getent()    { return 1; }
        curl()      { command -v curl >/dev/null && command curl \"\$@\" || return 0; }
        export -f id systemctl getent
        INSTALL_DIR='$FAKE_INSTALL_DIR'
        DATA_DIR='$FAKE_DATA_DIR'
        CONFIG_DIR='$FAKE_CONFIG_DIR'
        SERVICE_FILE='$SERVICE_FILE'
        export INSTALL_DIR DATA_DIR CONFIG_DIR SERVICE_FILE NO_COLOR
        _INSTALL_SH_TESTING=1 . '$SCRIPT'
        parse_maintenant_flags --uninstall
        check_prereqs
        uninstall
    "
    [ "$status" -eq 0 ]
    [ ! -f "$FAKE_INSTALL_DIR/maintenant" ]
    [ -d "$FAKE_DATA_DIR" ]
    [ -d "$FAKE_CONFIG_DIR" ]

    rm -rf "$FAKE_INSTALL_DIR" "$FAKE_DATA_DIR" "$FAKE_CONFIG_DIR" "$SERVICE_FILE"
}

@test "uninstall --purge: removes data dir and config dir" {
    FAKE_INSTALL_DIR=$(mktemp -d)
    FAKE_DATA_DIR=$(mktemp -d)
    FAKE_CONFIG_DIR=$(mktemp -d)
    SERVICE_FILE="/tmp/maintenant-test-purge-$$.service"
    touch "$SERVICE_FILE"

    run bash -c "
        NO_COLOR=1
        id()        { echo 0; }
        systemctl() { return 0; }
        getent()    { return 1; }
        userdel()   { return 0; }
        gpasswd()   { return 0; }
        export -f id systemctl getent userdel gpasswd
        INSTALL_DIR='$FAKE_INSTALL_DIR'
        DATA_DIR='$FAKE_DATA_DIR'
        CONFIG_DIR='$FAKE_CONFIG_DIR'
        SERVICE_FILE='$SERVICE_FILE'
        DO_PURGE=1
        export INSTALL_DIR DATA_DIR CONFIG_DIR SERVICE_FILE DO_PURGE NO_COLOR
        _INSTALL_SH_TESTING=1 . '$SCRIPT'
        parse_maintenant_flags --uninstall --purge
        check_prereqs
        uninstall
    "
    [ "$status" -eq 0 ]
    [ ! -d "$FAKE_DATA_DIR" ]
    [ ! -d "$FAKE_CONFIG_DIR" ]

    rm -rf "$FAKE_INSTALL_DIR" "$FAKE_DATA_DIR" "$FAKE_CONFIG_DIR" "$SERVICE_FILE" 2>/dev/null || true
}

@test "uninstall on system without maintenant returns 0 (idempotent)" {
    FAKE_INSTALL_DIR=$(mktemp -d)
    FAKE_DATA_DIR=$(mktemp -d)
    FAKE_CONFIG_DIR=$(mktemp -d)
    # No binary, no service file installed

    run bash -c "
        NO_COLOR=1
        id()        { echo 0; }
        systemctl() { return 0; }
        getent()    { return 1; }
        export -f id systemctl getent
        INSTALL_DIR='$FAKE_INSTALL_DIR'
        DATA_DIR='$FAKE_DATA_DIR'
        CONFIG_DIR='$FAKE_CONFIG_DIR'
        SERVICE_FILE='/tmp/no-such-service-$$.service'
        export INSTALL_DIR DATA_DIR CONFIG_DIR SERVICE_FILE NO_COLOR
        _INSTALL_SH_TESTING=1 . '$SCRIPT'
        parse_maintenant_flags --uninstall
        check_prereqs
        uninstall
    "
    [ "$status" -eq 0 ]

    rm -rf "$FAKE_INSTALL_DIR" "$FAKE_DATA_DIR" "$FAKE_CONFIG_DIR"
}
