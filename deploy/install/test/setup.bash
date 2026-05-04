#!/usr/bin/env bash
# Bats helper: mock commands used by install.sh
# Usage: load setup in each .bats file with: load 'setup'

# Mock curl: succeeds by default, writes fixture data if MOCK_CURL_DATA is set
mock_curl() {
    curl() {
        if [ -n "${MOCK_CURL_FAIL:-}" ]; then
            return "${MOCK_CURL_FAIL}"
        fi
        if [ -n "${MOCK_CURL_DATA:-}" ]; then
            printf '%s' "${MOCK_CURL_DATA}"
        fi
        return 0
    }
    export -f curl
}

# Mock systemctl: records calls, returns MOCK_SYSTEMCTL_RC (default 0)
mock_systemctl() {
    SYSTEMCTL_CALLS=()
    systemctl() {
        SYSTEMCTL_CALLS+=("$*")
        return "${MOCK_SYSTEMCTL_RC:-0}"
    }
    export -f systemctl
    export SYSTEMCTL_CALLS
}

# Mock useradd: records call, returns MOCK_USERADD_RC (default 0)
mock_useradd() {
    USERADD_CALLS=()
    useradd() {
        USERADD_CALLS+=("$*")
        return "${MOCK_USERADD_RC:-0}"
    }
    export -f useradd
    export USERADD_CALLS
}

# Mock cosign: succeeds by default, fails if MOCK_COSIGN_FAIL=1
mock_cosign() {
    cosign() {
        if [ -n "${MOCK_COSIGN_FAIL:-}" ]; then
            return 1
        fi
        return 0
    }
    export -f cosign
}

# Mock sha256sum: always succeeds (valid checksum) unless MOCK_SHA256_FAIL=1
mock_sha256sum() {
    sha256sum() {
        if [ -n "${MOCK_SHA256_FAIL:-}" ]; then
            echo "SHA256SUMS: FAILED"
            return 1
        fi
        return 0
    }
    export -f sha256sum
}

# Mock getent: group docker exists unless MOCK_NO_DOCKER_GROUP=1
mock_getent() {
    getent() {
        if [ "$1" = "group" ] && [ "$2" = "docker" ]; then
            if [ -n "${MOCK_NO_DOCKER_GROUP:-}" ]; then
                return 1
            fi
            echo "docker:x:999:maintenant"
            return 0
        fi
        return 1
    }
    export -f getent
}

# Provide a scratch TMPDIR for the test
setup_tmpdir() {
    export INSTALL_TMPDIR
    INSTALL_TMPDIR=$(mktemp -d)
}

teardown_tmpdir() {
    [ -d "${INSTALL_TMPDIR:-}" ] && rm -rf "$INSTALL_TMPDIR"
}
