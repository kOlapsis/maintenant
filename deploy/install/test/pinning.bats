#!/usr/bin/env bats
# Tests for version pinning (US4)
bats_require_minimum_version 1.5.0

load 'setup'

SCRIPT="$(cd "$(dirname "$BATS_TEST_FILENAME")/.." && pwd)/install.sh"

# Mock GitHub API response for a release with standalone binaries
MOCK_RELEASE_WITH_BINARIES='{
  "tag_name": "v1.5.0",
  "assets": [
    {"name": "maintenant-v1.5.0-linux-amd64", "size": 80000000},
    {"name": "maintenant-v1.5.0-linux-arm64", "size": 80000000},
    {"name": "SHA256SUMS", "size": 128}
  ]
}'

# Mock GitHub API response for a release WITHOUT standalone binaries (pre-v1.6)
MOCK_RELEASE_WITHOUT_BINARIES='{
  "tag_name": "v1.0.0",
  "assets": [
    {"name": "Source code (zip)", "size": 5000000}
  ]
}'

# Mock GitHub API list of 5 recent releases
MOCK_RELEASES_LIST='[
  {"tag_name": "v1.5.0"},
  {"tag_name": "v1.4.2"},
  {"tag_name": "v1.4.1"},
  {"tag_name": "v1.3.0"},
  {"tag_name": "v1.2.0"}
]'

@test "resolve_version: MAINTENANT_VERSION=latest resolves to latest tag" {
    result=$(bash -c "
        MAINTENANT_VERSION=latest
        NO_COLOR=1
        export MAINTENANT_VERSION NO_COLOR
        _INSTALL_SH_TESTING=1 . '$SCRIPT'
        log_info() { :; }; log_step() { :; }
        fetch_url() {
            case \"\$1\" in
                *releases/latest) echo '{\"tag_name\": \"v1.5.0\"}' ;;
                *) echo '[]' ;;
            esac
        }
        resolve_version
        echo \"\$VERSION\"
    ")
    [ "$result" = "v1.5.0" ]
}

@test "resolve_version: MAINTENANT_VERSION=v1.5.0 resolves exactly" {
    result=$(bash -c "
        MAINTENANT_VERSION=v1.5.0
        NO_COLOR=1
        export MAINTENANT_VERSION NO_COLOR
        _INSTALL_SH_TESTING=1 . '$SCRIPT'
        log_info() { :; }; log_step() { :; }
        fetch_url() {
            case \"\$1\" in
                *releases/tags/v1.5.0) printf '%s' '$MOCK_RELEASE_WITH_BINARIES' ;;
                *) echo '[]' ;;
            esac
        }
        resolve_version
        echo \"\$VERSION\"
    ")
    [ "$result" = "v1.5.0" ]
}

@test "resolve_version: non-existent version exits 20 with list of recent versions" {
    run bash -c "
        MAINTENANT_VERSION=v99.99.99
        NO_COLOR=1
        export MAINTENANT_VERSION NO_COLOR
        _INSTALL_SH_TESTING=1 . '$SCRIPT'
        fetch_url() {
            case \"\$1\" in
                *releases/tags/v99*) return 1 ;;
                *releases\?*) printf '%s' '$MOCK_RELEASES_LIST' ;;
                *) echo '{}' ;;
            esac
        }
        resolve_version
    "
    [ "$status" -eq 20 ]
}

@test "resolve_version: pre-standalone version exits 20 with helpful message" {
    run bash -c "
        MAINTENANT_VERSION=v1.0.0
        NO_COLOR=1
        export MAINTENANT_VERSION NO_COLOR
        _INSTALL_SH_TESTING=1 . '$SCRIPT'
        fetch_url() {
            case \"\$1\" in
                *releases/tags/v1.0.0) printf '%s' '$MOCK_RELEASE_WITHOUT_BINARIES' ;;
                *releases\?*) printf '%s' '$MOCK_RELEASES_LIST' ;;
                *) echo '{}' ;;
            esac
        }
        resolve_version
    " 2>&1
    [ "$status" -eq 20 ]
}
