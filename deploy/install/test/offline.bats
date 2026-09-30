#!/usr/bin/env bats
# Tests for the offline install from a local binary (--binary, --sha256sums)
bats_require_minimum_version 1.5.0

load 'setup'

SCRIPT="$(cd "$(dirname "$BATS_TEST_FILENAME")/.." && pwd)/install.sh"

setup() {
    WORK=$(mktemp -d)
    mkdir -p "$WORK/media" "$WORK/bin" "$WORK/data" "$WORK/etc"
    printf 'maintenant binary' > "$WORK/media/maintenant-v1.2.3-linux-amd64"
    (cd "$WORK/media" && sha256sum maintenant-v1.2.3-linux-amd64 > SHA256SUMS)
}

teardown() {
    rm -rf "$WORK"
}

# Every network path fails and leaves a trace in $WORK/network.log.
run_offline() {
    local svc
    svc=$(id -un)
    run bash -c "
        NO_COLOR=1
        uname() { case \"\$1\" in -s) echo Linux;; -m) echo x86_64;; esac; }
        id()   { echo '0'; }
        useradd() { return 0; }
        getent() { return 1; }
        chown() { return 0; }
        install() { cp \"\${@: -2:1}\" \"\${@: -1}\"; }
        curl() { echo \"curl \$*\" >> '$WORK/network.log'; return 7; }
        wget() { echo \"wget \$*\" >> '$WORK/network.log'; return 4; }
        export -f uname id useradd getent chown install curl wget
        INSTALL_DIR='$WORK/bin'
        DATA_DIR='$WORK/data'
        CONFIG_DIR='$WORK/etc'
        SERVICE_USER='$svc'
        export NO_COLOR INSTALL_DIR DATA_DIR CONFIG_DIR
        _INSTALL_SH_TESTING=1 . '$SCRIPT'
        fetch_url()    { echo \"fetch_url \$*\" >> '$WORK/network.log'; return 1; }
        fetch_url_to() { echo \"fetch_url_to \$*\" >> '$WORK/network.log'; return 1; }
        main --no-service $*
    "
}

@test "offline install: installs the local binary without any network access" {
    run_offline --binary "$WORK/media/maintenant-v1.2.3-linux-amd64" --sha256sums "$WORK/media/SHA256SUMS" --addr 0.0.0.0:8080
    [ "$status" -eq 0 ]
    [ ! -e "$WORK/network.log" ]
    [ "$(cat "$WORK/bin/maintenant")" = "maintenant binary" ]
    grep -qx 'MAINTENANT_ADDR=0.0.0.0:8080' "$WORK/etc/maintenant.env"
    [[ "$output" == *"Version : v1.2.3"* ]]
}

@test "offline install: a renamed binary is found in SHA256SUMS by its checksum" {
    cp "$WORK/media/maintenant-v1.2.3-linux-amd64" "$WORK/media/maintenant"
    run_offline --binary "$WORK/media/maintenant" --sha256sums "$WORK/media/SHA256SUMS"
    [ "$status" -eq 0 ]
    [[ "$output" == *"Version : v1.2.3"* ]]
}

@test "offline install: a checksum mismatch exits 21" {
    printf 'tampered' > "$WORK/media/maintenant-v1.2.3-linux-amd64"
    run_offline --binary "$WORK/media/maintenant-v1.2.3-linux-amd64" --sha256sums "$WORK/media/SHA256SUMS"
    [ "$status" -eq 21 ]
    [ ! -e "$WORK/bin/maintenant" ]
}

@test "offline install: a binary listed for another architecture exits 21" {
    (cd "$WORK/media" && sha256sum maintenant-v1.2.3-linux-amd64 \
        | sed 's/linux-amd64/linux-arm64/' > SHA256SUMS)
    run_offline --binary "$WORK/media/maintenant-v1.2.3-linux-amd64" --sha256sums "$WORK/media/SHA256SUMS"
    [ "$status" -eq 21 ]
}

@test "offline install: a missing binary exits 2" {
    run_offline --binary "$WORK/media/nope"
    [ "$status" -eq 2 ]
}

@test "offline install: without --sha256sums it warns and installs" {
    run_offline --binary "$WORK/media/maintenant-v1.2.3-linux-amd64"
    [ "$status" -eq 0 ]
    [[ "$output" == *"integrity of"*"is not checked"* ]]
    [ ! -e "$WORK/network.log" ]
}

@test "check_prereqs: an offline install needs neither curl nor wget" {
    run bash -c "
        id() { echo '0'; }
        install() { :; }
        useradd() { :; }
        export -f id install useradd
        PATH=/usr/bin/no-such-dir
        _INSTALL_SH_TESTING=1 NO_COLOR=1 . '$SCRIPT'
        parse_maintenant_flags --no-service --binary ./maintenant
        check_prereqs
    "
    [ "$status" -eq 0 ]
}
