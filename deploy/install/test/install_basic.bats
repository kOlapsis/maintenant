#!/usr/bin/env bats
# Tests for basic install flow (US1)
bats_require_minimum_version 1.5.0

load 'setup'

SCRIPT="$(cd "$(dirname "$BATS_TEST_FILENAME")/.." && pwd)/install.sh"

# ── Helpers ───────────────────────────────────────────────────────────────────

run_script() {
    # Source the script functions into a controlled environment
    # We use env -i to avoid inheriting real system state
    bash -c "
        NO_COLOR=1
        export NO_COLOR
        # shellcheck source=/dev/null
        . '$SCRIPT'
        $*
    "
}

# ── detect_platform ───────────────────────────────────────────────────────────

@test "detect_platform: x86_64 maps to amd64" {
    result=$(bash -c "
        uname() { case \"\$1\" in -s) echo Linux;; -m) echo x86_64;; esac; }
        export -f uname
        NO_COLOR=1 . '$SCRIPT'
        detect_platform
        echo \"\$ARCH\"
    ")
    [ "$result" = "amd64" ]
}

@test "detect_platform: aarch64 maps to arm64" {
    result=$(bash -c "
        uname() { case \"\$1\" in -s) echo Linux;; -m) echo aarch64;; esac; }
        export -f uname
        NO_COLOR=1 . '$SCRIPT'
        detect_platform
        echo \"\$ARCH\"
    ")
    [ "$result" = "arm64" ]
}

@test "detect_platform: armv7l is rejected with exit 10" {
    run bash -c "
        uname() { case \"\$1\" in -s) echo Linux;; -m) echo armv7l;; esac; }
        export -f uname
        NO_COLOR=1 . '$SCRIPT'
        detect_platform
    "
    [ "$status" -eq 10 ]
}

@test "detect_platform: non-linux OS is rejected with exit 10" {
    run bash -c "
        uname() { case \"\$1\" in -s) echo Darwin;; -m) echo x86_64;; esac; }
        export -f uname
        NO_COLOR=1 . '$SCRIPT'
        detect_platform
    "
    [ "$status" -eq 10 ]
}

# ── check_prereqs ─────────────────────────────────────────────────────────────

@test "check_prereqs: exits 11 when not root" {
    run bash -c "
        id() { echo '1000'; }
        export -f id
        NO_COLOR=1 . '$SCRIPT'
        check_prereqs
    "
    [ "$status" -eq 11 ]
}

@test "check_prereqs: exits 12 when curl and wget both missing" {
    run bash -c "
        id() { echo '0'; }
        export -f id
        # Remove curl and wget from PATH
        PATH=/usr/bin/no-such-dir
        NO_COLOR=1 . '$SCRIPT'
        check_prereqs
    "
    [ "$status" -eq 12 ]
}

# ── download_and_verify ───────────────────────────────────────────────────────

@test "download_and_verify: passes with valid checksum" {
    FAKE_TMPDIR=$(mktemp -d)
    # Create a fake binary and valid SHA256SUMS
    printf 'fakebinary' > "$FAKE_TMPDIR/maintenant-v1.0.0-linux-amd64"
    (cd "$FAKE_TMPDIR" && sha256sum maintenant-v1.0.0-linux-amd64 > SHA256SUMS)

    run bash -c "
        # Mock fetch functions
        fetch_url_to() { cp '$FAKE_TMPDIR/\$(basename \"\$1\")' \"\$2\" 2>/dev/null || true; }
        export -f fetch_url_to
        TMPDIR_INSTALL='$FAKE_TMPDIR'
        VERSION='v1.0.0'
        ARCH='amd64'
        SKIP_COSIGN=1
        NO_COLOR=1
        export TMPDIR_INSTALL VERSION ARCH SKIP_COSIGN NO_COLOR
        . '$SCRIPT'
        download_and_verify
    "
    [ "$status" -eq 0 ]
    rm -rf "$FAKE_TMPDIR"
}

@test "download_and_verify: exits 21 on checksum mismatch" {
    FAKE_TMPDIR=$(mktemp -d)
    printf 'fakebinary' > "$FAKE_TMPDIR/maintenant-v1.0.0-linux-amd64"
    printf 'deadbeef  maintenant-v1.0.0-linux-amd64\n' > "$FAKE_TMPDIR/SHA256SUMS"

    run bash -c "
        fetch_url_to() { cp '$FAKE_TMPDIR/\$(basename \"\$1\")' \"\$2\" 2>/dev/null || true; }
        export -f fetch_url_to
        TMPDIR_INSTALL='$FAKE_TMPDIR'
        VERSION='v1.0.0'
        ARCH='amd64'
        SKIP_COSIGN=1
        NO_COLOR=1
        export TMPDIR_INSTALL VERSION ARCH SKIP_COSIGN NO_COLOR
        . '$SCRIPT'
        download_and_verify
    "
    [ "$status" -eq 21 ]
    rm -rf "$FAKE_TMPDIR"
}

# ── full flow --no-service ────────────────────────────────────────────────────

@test "full install with --no-service succeeds" {
    FAKE_TMPDIR=$(mktemp -d)
    FAKE_INSTALL_DIR=$(mktemp -d)
    FAKE_DATA_DIR=$(mktemp -d)
    FAKE_CONFIG_DIR=$(mktemp -d)

    printf 'fakebinary' > "$FAKE_TMPDIR/maintenant-v1.0.0-linux-amd64"
    (cd "$FAKE_TMPDIR" && sha256sum maintenant-v1.0.0-linux-amd64 > SHA256SUMS)

    run bash -c "
        # Mock system commands
        uname() { case \"\$1\" in -s) echo Linux;; -m) echo x86_64;; esac; }
        id()   { echo '0'; }
        useradd() { return 0; }
        getent() { return 1; }
        fetch_url_to() { cp '$FAKE_TMPDIR/\$(basename \"\$1\")' \"\$2\" 2>/dev/null || true; }
        fetch_url()    { echo '{\"tag_name\": \"v1.0.0\"}'; }
        chown() { return 0; }
        chmod() { return 0; }
        export -f uname id useradd getent fetch_url_to fetch_url chown chmod
        INSTALL_DIR='$FAKE_INSTALL_DIR'
        DATA_DIR='$FAKE_DATA_DIR'
        CONFIG_DIR='$FAKE_CONFIG_DIR'
        MAINTENANT_VERSION='v1.0.0'
        SKIP_COSIGN=1
        NO_COLOR=1
        export INSTALL_DIR DATA_DIR CONFIG_DIR MAINTENANT_VERSION SKIP_COSIGN NO_COLOR
        . '$SCRIPT'
        main --no-service
    "
    [ "$status" -eq 0 ]
    rm -rf "$FAKE_TMPDIR" "$FAKE_INSTALL_DIR" "$FAKE_DATA_DIR" "$FAKE_CONFIG_DIR"
}
