#!/usr/bin/env bats
# Tests for the binary swap, the systemd unit and the directories it may write
bats_require_minimum_version 1.5.0

load 'setup'

SCRIPT="$(cd "$(dirname "$BATS_TEST_FILENAME")/.." && pwd)/install.sh"
UNIT="$(cd "$(dirname "$BATS_TEST_FILENAME")/.." && pwd)/maintenant.service"

setup() {
    WORK=$(mktemp -d)
}

teardown() {
    rm -rf "$WORK"
}

run_install_service() {
    run bash -c "
        NO_COLOR=1
        SERVICE_FILE='$WORK/maintenant.service'
        export NO_COLOR SERVICE_FILE
        systemctl() {
            echo \"\$*\" >> '$WORK/systemctl.log'
            case \"\$*\" in
                start*|restart*|'enable --now'*) touch '$WORK/active' ;;
                is-active*) [ -e '$WORK/active' ] ;;
                show*) echo '${NRESTARTS:-0}' ;;
            esac
        }
        sleep() { [ -z '${CRASH_AFTER_START:-}' ] || rm -f '$WORK/active'; }
        journalctl() { :; }
        export -f systemctl sleep journalctl
        [ -z '${ALREADY_ACTIVE:-}' ] || touch '$WORK/active'
        _INSTALL_SH_TESTING=1 . '$SCRIPT'
        DATA_DIR=/var/lib/maintenant
        DB_DIR=/var/lib/maintenant
        INSTALL_DIR='$WORK/bin'
        [ ! -e '$WORK/bin/.maintenant.previous' ] || BINARY_PREVIOUS='$WORK/bin/.maintenant.previous'
        install_service
    "
}

upgrade_in_place() {
    mkdir -p "$WORK/bin"
    printf 'new' > "$WORK/bin/maintenant"
    printf 'old' > "$WORK/bin/.maintenant.previous"
}

@test "install_service: an upgrade that stays up drops the previous binary" {
    upgrade_in_place
    ALREADY_ACTIVE=1 run_install_service
    [ "$status" -eq 0 ]
    [ "$(cat "$WORK/bin/maintenant")" = "new" ]
    [ "$(ls -A "$WORK/bin")" = "maintenant" ]
}

@test "install_service: an upgrade that crashes puts the previous binary back and restarts it" {
    upgrade_in_place
    ALREADY_ACTIVE=1 CRASH_AFTER_START=1 run_install_service
    [ "$status" -eq 31 ]
    [ "$(cat "$WORK/bin/maintenant")" = "old" ]
    [ "$(ls -A "$WORK/bin")" = "maintenant" ]
    [ "$(grep -c '^restart maintenant$' "$WORK/systemctl.log")" -eq 2 ]
    [[ "$output" == *"previous binary was put back"* ]]
    [[ "$output" != *"Maintenant is installed"* ]]
}

@test "install_service: an upgrade that systemd already restarted is rolled back too" {
    upgrade_in_place
    ALREADY_ACTIVE=1 NRESTARTS=2 run_install_service
    [ "$status" -eq 31 ]
    [ "$(cat "$WORK/bin/maintenant")" = "old" ]
}

@test "install_service: a failing restart on an upgrade puts the previous binary back" {
    upgrade_in_place
    run bash -c "
        NO_COLOR=1
        SERVICE_FILE='$WORK/maintenant.service'
        export NO_COLOR SERVICE_FILE
        systemctl() {
            case \"\$*\" in
                restart*) return 1 ;;
            esac
            return 0
        }
        journalctl() { :; }
        export -f systemctl journalctl
        _INSTALL_SH_TESTING=1 . '$SCRIPT'
        DATA_DIR=/var/lib/maintenant
        DB_DIR=/var/lib/maintenant
        INSTALL_DIR='$WORK/bin'
        BINARY_PREVIOUS='$WORK/bin/.maintenant.previous'
        install_service
    "
    [ "$status" -eq 31 ]
    [ "$(cat "$WORK/bin/maintenant")" = "old" ]
    [[ "$output" == *"could not be restarted"* ]]
}

@test "install_service: --no-service drops the previous binary" {
    upgrade_in_place
    NO_SERVICE=1 run_install_service
    [ "$status" -eq 0 ]
    [ "$(cat "$WORK/bin/maintenant")" = "new" ]
    [ "$(ls -A "$WORK/bin")" = "maintenant" ]
}

@test "install_service: a first install that crashes has nothing to put back" {
    mkdir -p "$WORK/bin"
    printf 'new' > "$WORK/bin/maintenant"
    CRASH_AFTER_START=1 run_install_service
    [ "$status" -eq 31 ]
    [ "$(cat "$WORK/bin/maintenant")" = "new" ]
    [[ "$output" == *"does not stay up"* ]]
    [[ "$output" != *"previous binary"* ]]
}

@test "install_service: restarts a service that is already running" {
    ALREADY_ACTIVE=1 run_install_service
    [ "$status" -eq 0 ]
    grep -qx 'restart maintenant' "$WORK/systemctl.log"
}

@test "install_service: starts a service that is not running" {
    run_install_service
    [ "$status" -eq 0 ]
    grep -qx 'start maintenant' "$WORK/systemctl.log"
    [ -z "$(grep '^restart' "$WORK/systemctl.log")" ]
}

@test "install_service: a service that dies right after its start exits 31" {
    CRASH_AFTER_START=1 run_install_service
    [ "$status" -eq 31 ]
    [[ "$output" == *"Last 50 log lines"* ]]
}

@test "install_service: a service that systemd already restarted exits 31" {
    NRESTARTS=1 run_install_service
    [ "$status" -eq 31 ]
    [[ "$output" == *"Last 50 log lines"* ]]
}

@test "install_service: --no-service warns when the running service keeps the old binary" {
    NO_SERVICE=1 ALREADY_ACTIVE=1 run_install_service
    [ "$status" -eq 0 ]
    [[ "$output" == *"still runs the previous binary"* ]]
    [ -z "$(grep -E '^(start|restart|enable)' "$WORK/systemctl.log")" ]
}

@test "install_service: a failing restart exits 31" {
    run bash -c "
        NO_COLOR=1
        SERVICE_FILE='$WORK/maintenant.service'
        export NO_COLOR SERVICE_FILE
        systemctl() {
            case \"\$*\" in
                restart*) return 1 ;;
            esac
            return 0
        }
        export -f systemctl
        _INSTALL_SH_TESTING=1 . '$SCRIPT'
        DATA_DIR=/var/lib/maintenant
        DB_DIR=/var/lib/maintenant
        install_service
    "
    [ "$status" -eq 31 ]
}

summary_for() {
    run bash -c "
        MAINTENANT_ADDR=10.9.9.9:1
        export MAINTENANT_ADDR
        _INSTALL_SH_TESTING=1 NO_COLOR=1 . '$SCRIPT'
        CONFIG_DIR='$WORK/etc'
        VERSION=v1.0.0
        parse_maintenant_flags $*
        print_summary
    "
}

@test "print_summary: shows the address given as a flag" {
    mkdir -p "$WORK/etc"
    printf 'MAINTENANT_ADDR=0.0.0.0:7000\n' > "$WORK/etc/maintenant.env"
    summary_for --addr 0.0.0.0:9000
    [ "$status" -eq 0 ]
    [[ "$output" == *"Listens : http://0.0.0.0:9000"$'\n'* ]]
}

@test "print_summary: shows the address already in the env file" {
    mkdir -p "$WORK/etc"
    printf 'MAINTENANT_ADDR="0.0.0.0:7000"\n' > "$WORK/etc/maintenant.env"
    summary_for --logLevel debug
    [ "$status" -eq 0 ]
    [[ "$output" == *"Listens : http://0.0.0.0:7000"$'\n'* ]]
}

@test "print_summary: falls back to the binary's default address" {
    summary_for
    [ "$status" -eq 0 ]
    [[ "$output" == *"Listens : http://127.0.0.1:8080"$'\n'* ]]
}

@test "install_binary: writes a temporary file next to the binary, then renames it" {
    mkdir -p "$WORK/bin" "$WORK/tmp"
    printf 'old' > "$WORK/bin/maintenant"
    printf 'new' > "$WORK/tmp/maintenant-v1.0.0-linux-amd64"

    run bash -c "
        NO_COLOR=1
        export NO_COLOR
        install() {
            echo \"\${@: -1}\" >> '$WORK/install.log'
            cp \"\${@: -2:1}\" \"\${@: -1}\"
        }
        export -f install
        _INSTALL_SH_TESTING=1 . '$SCRIPT'
        INSTALL_DIR='$WORK/bin'
        TMPDIR_INSTALL='$WORK/tmp'
        VERSION=v1.0.0
        ARCH=amd64
        BINARY_SRC='$WORK/tmp/maintenant-v1.0.0-linux-amd64'
        install_binary
    "
    [ "$status" -eq 0 ]
    [ "$(cat "$WORK/bin/maintenant")" = "new" ]
    written=$(cat "$WORK/install.log")
    [ "$(dirname "$written")" = "$WORK/bin" ]
    [ "$written" != "$WORK/bin/maintenant" ]
    [ "$(cat "$WORK/bin/.maintenant.previous")" = "old" ]
    [ "$(ls -A "$WORK/bin" | wc -l)" -eq 2 ]
}

@test "install_binary: a first install keeps no previous binary" {
    mkdir -p "$WORK/bin" "$WORK/tmp"
    printf 'new' > "$WORK/tmp/maintenant-v1.0.0-linux-amd64"

    run bash -c "
        NO_COLOR=1
        export NO_COLOR
        install() { cp \"\${@: -2:1}\" \"\${@: -1}\"; }
        export -f install
        _INSTALL_SH_TESTING=1 . '$SCRIPT'
        INSTALL_DIR='$WORK/bin'
        BINARY_SRC='$WORK/tmp/maintenant-v1.0.0-linux-amd64'
        install_binary
        printf 'previous=[%s]' \"\$BINARY_PREVIOUS\"
    "
    [ "$status" -eq 0 ]
    [[ "$output" == *"previous=[]"* ]]
    [ "$(ls -A "$WORK/bin")" = "maintenant" ]
}

@test "the reference unit is what the script writes with its defaults" {
    run bash -c "
        unset DATA_DIR MAINTENANT_DATA_DIR CONFIG_DIR MAINTENANT_CONFIG_DIR INSTALL_DIR MAINTENANT_INSTALL_DIR SERVICE_USER
        _INSTALL_SH_TESTING=1 . '$SCRIPT'
        _env_file_value() { :; }
        resolve_paths
        render_unit
    "
    [ "$status" -eq 0 ]
    [ "$output" = "$(cat "$UNIT")" ]
}

render_for() {
    run bash -c "
        unset DATA_DIR MAINTENANT_DATA_DIR
        _INSTALL_SH_TESTING=1 NO_COLOR=1 . '$SCRIPT'
        CONFIG_DIR='$WORK/etc'
        parse_maintenant_flags $*
        resolve_paths
        render_unit
    "
}

@test "unit: --db outside the data dir is writable" {
    render_for --db /srv/maintenant-db/maintenant.db
    [ "$status" -eq 0 ]
    [[ "$output" == *"ReadWritePaths=/var/lib/maintenant /srv/maintenant-db"$'\n'* ]]
}

@test "unit: --data-dir moves the working directory, the writable path, MAINTENANT_DATA_DIR and SQLITE_TMPDIR" {
    render_for --data-dir /srv/maintenant/
    [ "$status" -eq 0 ]
    [[ "$output" == *"WorkingDirectory=/srv/maintenant"$'\n'* ]]
    [[ "$output" == *"ReadWritePaths=/srv/maintenant"$'\n'* ]]
    [[ "$output" == *"Environment=MAINTENANT_DATA_DIR=/srv/maintenant"$'\n'* ]]
    [[ "$output" == *"Environment=SQLITE_TMPDIR=/srv/maintenant"$'\n'* ]]
}

@test "unit: a relative --db lives in the data dir" {
    render_for --data-dir /srv/maintenant --db ./db/maintenant.db
    [ "$status" -eq 0 ]
    [[ "$output" == *"ReadWritePaths=/srv/maintenant /srv/maintenant/db"$'\n'* ]]
}

@test "unit: paths already in the env file are kept on a rerun without flags" {
    mkdir -p "$WORK/etc"
    printf 'MAINTENANT_DATA_DIR="/srv/agent"\nMAINTENANT_DB=/srv/db/maintenant.db\n' > "$WORK/etc/maintenant.env"
    render_for --logLevel debug
    [ "$status" -eq 0 ]
    [[ "$output" == *"WorkingDirectory=/srv/agent"$'\n'* ]]
    [[ "$output" == *"ReadWritePaths=/srv/agent /srv/db"$'\n'* ]]
}

@test "resolve_paths: a relative data dir exits 2" {
    render_for --data-dir data
    [ "$status" -eq 2 ]
}

@test "resolve_paths: a path the unit file cannot carry exits 2" {
    render_for --db '"/srv/my db/maintenant.db"'
    [ "$status" -eq 2 ]
}

@test "prepare_dirs: creates the data and database directories for the service user" {
    run bash -c "
        NO_COLOR=1
        export NO_COLOR
        chown() { echo \"\$*\" >> '$WORK/chown.log'; }
        export -f chown
        _INSTALL_SH_TESTING=1 . '$SCRIPT'
        SERVICE_USER=\$(id -un)
        CONFIG_DIR='$WORK/etc'
        parse_maintenant_flags --data-dir '$WORK/data' --db '$WORK/db/maintenant.db'
        resolve_paths
        prepare_dirs
    "
    [ "$status" -eq 0 ]
    [ -d "$WORK/data" ]
    [ -d "$WORK/db" ]
    grep -q ":.* $WORK/data\$" "$WORK/chown.log"
    grep -q ":.* $WORK/db\$" "$WORK/chown.log"
}

@test "prepare_dirs: refuses a non-empty directory that belongs to someone else" {
    mkdir -p "$WORK/shared"
    touch "$WORK/shared/other-app.conf"
    run bash -c "
        NO_COLOR=1
        export NO_COLOR
        chown() { echo \"\$*\" >> '$WORK/chown.log'; }
        export -f chown
        _INSTALL_SH_TESTING=1 . '$SCRIPT'
        SERVICE_USER=nobody
        CONFIG_DIR='$WORK/etc'
        parse_maintenant_flags --data-dir '$WORK/data' --db '$WORK/shared/maintenant.db'
        resolve_paths
        prepare_dirs
    "
    [ "$status" -eq 30 ]
    [ ! -e "$WORK/chown.log" ]
}
