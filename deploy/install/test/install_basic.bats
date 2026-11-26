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
        _INSTALL_SH_TESTING=1 . '$SCRIPT'
        $*
    "
}

# ── detect_platform ───────────────────────────────────────────────────────────

@test "detect_platform: x86_64 maps to amd64" {
    result=$(bash -c "
        uname() { case \"\$1\" in -s) echo Linux;; -m) echo x86_64;; esac; }
        export -f uname
        _INSTALL_SH_TESTING=1 NO_COLOR=1 . '$SCRIPT'
        detect_platform
        echo \"\$ARCH\"
    ")
    [ "$result" = "amd64" ]
}

@test "detect_platform: aarch64 maps to arm64" {
    result=$(bash -c "
        uname() { case \"\$1\" in -s) echo Linux;; -m) echo aarch64;; esac; }
        export -f uname
        _INSTALL_SH_TESTING=1 NO_COLOR=1 . '$SCRIPT'
        detect_platform
        echo \"\$ARCH\"
    ")
    [ "$result" = "arm64" ]
}

@test "detect_platform: armv7l is rejected with exit 10" {
    run bash -c "
        uname() { case \"\$1\" in -s) echo Linux;; -m) echo armv7l;; esac; }
        export -f uname
        _INSTALL_SH_TESTING=1 NO_COLOR=1 . '$SCRIPT'
        detect_platform
    "
    [ "$status" -eq 10 ]
}

@test "detect_platform: non-linux OS is rejected with exit 10" {
    run bash -c "
        uname() { case \"\$1\" in -s) echo Darwin;; -m) echo x86_64;; esac; }
        export -f uname
        _INSTALL_SH_TESTING=1 NO_COLOR=1 . '$SCRIPT'
        detect_platform
    "
    [ "$status" -eq 10 ]
}

# ── check_prereqs ─────────────────────────────────────────────────────────────

@test "check_prereqs: exits 11 when not root" {
    run bash -c "
        id() { echo '1000'; }
        export -f id
        _INSTALL_SH_TESTING=1 NO_COLOR=1 . '$SCRIPT'
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
        _INSTALL_SH_TESTING=1 NO_COLOR=1 . '$SCRIPT'
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
        VERSION='v1.0.0'
        ARCH='amd64'
        SKIP_COSIGN=1
        NO_COLOR=1
        export VERSION ARCH SKIP_COSIGN NO_COLOR
        _INSTALL_SH_TESTING=1 . '$SCRIPT'
        fetch_url_to() { cp '$FAKE_TMPDIR/'\"\$(basename \"\$2\")\" \"\$2\" 2>/dev/null || true; }
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
        VERSION='v1.0.0'
        ARCH='amd64'
        SKIP_COSIGN=1
        NO_COLOR=1
        export VERSION ARCH SKIP_COSIGN NO_COLOR
        _INSTALL_SH_TESTING=1 . '$SCRIPT'
        fetch_url_to() { cp '$FAKE_TMPDIR/'\"\$(basename \"\$2\")\" \"\$2\" 2>/dev/null || true; }
        download_and_verify
    "
    [ "$status" -eq 21 ]
    rm -rf "$FAKE_TMPDIR"
}

@test "download_and_verify: a failed download exits 20" {
    run bash -c "
        VERSION='v1.0.0'
        ARCH='amd64'
        SKIP_COSIGN=1
        NO_COLOR=1
        export VERSION ARCH SKIP_COSIGN NO_COLOR
        _INSTALL_SH_TESTING=1 . '$SCRIPT'
        fetch_url_to() { return 22; }
        download_and_verify
    "
    [ "$status" -eq 20 ]
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
        # Mock system commands (before source — these don't conflict with script functions)
        uname() { case \"\$1\" in -s) echo Linux;; -m) echo x86_64;; esac; }
        id()   { echo '0'; }
        useradd() { return 0; }
        getent() { return 1; }
        chown() { return 0; }
        chmod() { return 0; }
        install() { return 0; }
        export -f uname id useradd getent chown chmod install
        INSTALL_DIR='$FAKE_INSTALL_DIR'
        DATA_DIR='$FAKE_DATA_DIR'
        CONFIG_DIR='$FAKE_CONFIG_DIR'
        MAINTENANT_VERSION='v1.0.0'
        SKIP_COSIGN=1
        NO_COLOR=1
        export INSTALL_DIR DATA_DIR CONFIG_DIR MAINTENANT_VERSION SKIP_COSIGN NO_COLOR
        _INSTALL_SH_TESTING=1 . '$SCRIPT'
        # Mock fetch functions after source — source overwrites them otherwise
        fetch_url_to() { cp '$FAKE_TMPDIR/'\"\$(basename \"\$2\")\" \"\$2\" 2>/dev/null || true; }
        fetch_url()    { echo '{\"tag_name\": \"v1.0.0\", \"assets\": [{\"name\": \"maintenant-v1.0.0-linux-amd64\", \"size\": 10}]}'; }
        main --no-service
    "
    [ "$status" -eq 0 ]
    rm -rf "$FAKE_TMPDIR" "$FAKE_INSTALL_DIR" "$FAKE_DATA_DIR" "$FAKE_CONFIG_DIR"
}

# ── cosign version gate ───────────────────────────────────────────────────────

@test "_cosign_major: reads the major from cosign version output" {
    result=$(bash -c "
        cosign() { echo '  GitVersion:    v3.1.2'; }
        export -f cosign
        _INSTALL_SH_TESTING=1 . '$SCRIPT'
        _cosign_major
    ")
    [ "$result" = "3" ]
}

@test "_cosign_major: empty when the version cannot be read" {
    result=$(bash -c "
        cosign() { echo 'not a version'; }
        export -f cosign
        _INSTALL_SH_TESTING=1 . '$SCRIPT'
        _cosign_major
    ")
    [ -z "$result" ]
}

NO_COSIGN='command() { [ "$2" != cosign ] && builtin command "$@"; }'
COSIGN_2="cosign() { echo '  GitVersion:    v2.4.1'; }; export -f cosign"
COSIGN_3_BAD="cosign() { [ \"\$1\" = version ] && echo '  GitVersion:    v3.0.2'; }; export -f cosign"
COSIGN_3_OK="cosign() { echo '  GitVersion:    v3.0.2'; }; export -f cosign"

verify_then_summary() {
    FAKE_TMPDIR=$(mktemp -d)
    printf 'fakebinary' > "$FAKE_TMPDIR/maintenant-v1.0.0-linux-amd64"
    (cd "$FAKE_TMPDIR" && sha256sum maintenant-v1.0.0-linux-amd64 > SHA256SUMS)
    touch "$FAKE_TMPDIR/SHA256SUMS.bundle"
    setup_env="$1"
    shift

    run bash -c "
        NO_COLOR=1; export NO_COLOR
        unset SKIP_COSIGN MAINTENANT_SKIP_COSIGN REQUIRE_COSIGN MAINTENANT_REQUIRE_COSIGN
        $setup_env
        _INSTALL_SH_TESTING=1 . '$SCRIPT'
        VERSION=v1.0.0
        ARCH=amd64
        CONFIG_DIR='$FAKE_TMPDIR/etc'
        fetch_url_to() { cp '$FAKE_TMPDIR/'\"\$(basename \"\$2\")\" \"\$2\" 2>/dev/null || true; }
        parse_maintenant_flags $*
        download_and_verify
        print_summary
    "
    rm -rf "$FAKE_TMPDIR"
}

@test "cosign 2: installs, and the summary says the signature was not verified" {
    verify_then_summary "$COSIGN_2"
    [ "$status" -eq 0 ]
    [[ "$output" == *"cosign 3 or later is required"* ]]
    [[ "$output" == *"installed successfully"*"SIGNATURE NOT VERIFIED"* ]]
    [[ "$output" == *"cosign verify-blob --bundle SHA256SUMS.bundle"* ]]
    [[ "$output" == *"releases/download/v1.0.0/SHA256SUMS"* ]]
    [[ "$output" == *"maintenant-v1.0.0-linux-amd64"* ]]
    [[ "$output" == *"--require-cosign"* ]]
}

@test "no cosign: installs, and the summary says the signature was not verified" {
    verify_then_summary "$NO_COSIGN"
    [ "$status" -eq 0 ]
    [[ "$output" == *"installed successfully"*"SIGNATURE NOT VERIFIED"*"cosign is not installed"* ]]
    [[ "$output" == *"--require-cosign"* ]]
}

@test "--require-cosign: no cosign exits 22" {
    verify_then_summary "$NO_COSIGN" --require-cosign
    [ "$status" -eq 22 ]
    [[ "$output" != *"installed successfully"* ]]
}

@test "--require-cosign: cosign 2 exits 22" {
    verify_then_summary "$COSIGN_2" --require-cosign
    [ "$status" -eq 22 ]
}

@test "MAINTENANT_REQUIRE_COSIGN behaves like --require-cosign" {
    verify_then_summary "$NO_COSIGN; MAINTENANT_REQUIRE_COSIGN=1"
    [ "$status" -eq 22 ]
}

@test "--require-cosign with --skip-cosign exits 2" {
    verify_then_summary "$COSIGN_3_OK" --require-cosign --skip-cosign
    [ "$status" -eq 2 ]
}

@test "cosign 3: a signature that does not verify exits 22 without any flag" {
    verify_then_summary "$COSIGN_3_BAD"
    [ "$status" -eq 22 ]
}

@test "cosign 3: a verified signature leaves no warning in the summary" {
    verify_then_summary "$COSIGN_3_OK" --require-cosign
    [ "$status" -eq 0 ]
    [[ "$output" == *"cosign signature verified"* ]]
    [[ "$output" != *"SIGNATURE NOT VERIFIED"* ]]
}

@test "--skip-cosign: installs without cosign and keeps the summary plain" {
    verify_then_summary "$NO_COSIGN" --skip-cosign
    [ "$status" -eq 0 ]
    [[ "$output" == *"cosign verification skipped"* ]]
    [[ "$output" != *"SIGNATURE NOT VERIFIED"* ]]
}

@test "GITHUB_REPO matches the repository spelling used in the certificate identity" {
    # Sigstore matches the SAN case-sensitively. A lowercase spelling still
    # downloads (GitHub URLs are case-insensitive) and then fails verification
    # on every user machine, which is exactly what happened in v1.4.0.
    result=$(bash -c "_INSTALL_SH_TESTING=1 . '$SCRIPT'; printf '%s' \"\$GITHUB_REPO\"")
    [ "$result" = "kOlapsis/maintenant" ]
}
