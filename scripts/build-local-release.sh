#!/usr/bin/env sh
# Build a local "release set" for VM testing without ever publishing on GitHub.
# Produces:
#   dist/release/<VERSION>/{maintenant-<VERSION>-linux-<ARCH>, install.sh, SHA256SUMS}
#   dist/api/repos/kolapsis/maintenant/releases/{latest, tags/<VERSION>}
# Then serve `dist/` on the LAN and point a VM at it via env hooks.
# See specs/018-install-script-cli-args/quickstart.md (section "Test pré-release VM").

set -eu

# ── Config (env-overridable) ──────────────────────────────────────────────────
VERSION="${VERSION:-v0.0.0-local}"
ARCH="${ARCH:-$(go env GOARCH 2>/dev/null || uname -m)}"
DIST_ROOT="${DIST_ROOT:-dist}"
SKIP_BUILD="${SKIP_BUILD:-}"

# ── Paths ─────────────────────────────────────────────────────────────────────
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
RELEASE_DIR="${REPO_ROOT}/${DIST_ROOT}/release/${VERSION}"
API_DIR="${REPO_ROOT}/${DIST_ROOT}/api/repos/kolapsis/maintenant/releases"

cd "$REPO_ROOT"

# ── Detect git SHA ────────────────────────────────────────────────────────────
SHORT_SHA="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
COMMIT="$(git rev-parse HEAD 2>/dev/null || echo unknown)"
BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

# ── Map arch ──────────────────────────────────────────────────────────────────
case "$ARCH" in
    x86_64|amd64) ARCH=amd64 ;;
    aarch64|arm64) ARCH=arm64 ;;
    *) echo "Unsupported arch: $ARCH (set ARCH=amd64 or ARCH=arm64)" >&2; exit 1 ;;
esac

ASSET_NAME="maintenant-${VERSION}-linux-${ARCH}"

# ── Reset API mock (always); reset release dir only when rebuilding ───────────
rm -rf "$API_DIR"
mkdir -p "$API_DIR/tags"

if [ -z "$SKIP_BUILD" ]; then
    rm -rf "$RELEASE_DIR"
fi
mkdir -p "$RELEASE_DIR"

# ── Build binary ──────────────────────────────────────────────────────────────
if [ -z "$SKIP_BUILD" ]; then
    echo "==> Building binary ($ASSET_NAME) for linux/$ARCH..."
    CGO_ENABLED=1 GOOS=linux GOARCH="$ARCH" \
        go build \
            -ldflags "-s -w \
                -X main.version=${VERSION} \
                -X main.commit=${COMMIT} \
                -X main.buildDate=${BUILD_DATE}" \
            -o "$RELEASE_DIR/$ASSET_NAME" \
            ./cmd/maintenant
    echo "    Built $RELEASE_DIR/$ASSET_NAME"
else
    echo "==> SKIP_BUILD=1 — expecting existing binary at $RELEASE_DIR/$ASSET_NAME"
    if [ ! -f "$RELEASE_DIR/$ASSET_NAME" ]; then
        echo "    Missing binary at $RELEASE_DIR/$ASSET_NAME — clear SKIP_BUILD or place one there." >&2
        exit 1
    fi
fi

# ── Render install.sh with local SHA ──────────────────────────────────────────
echo "==> Rendering install.sh with SCRIPT_VERSION=local-${SHORT_SHA}..."
sed "s/__GIT_SHA__/local-${SHORT_SHA}/" \
    deploy/install/install.sh > "$RELEASE_DIR/install.sh"
chmod +x "$RELEASE_DIR/install.sh"
if grep -q '__GIT_SHA__' "$RELEASE_DIR/install.sh"; then
    echo "    sed failed: placeholder still present" >&2
    exit 1
fi

# ── Generate SHA256SUMS ───────────────────────────────────────────────────────
echo "==> Generating SHA256SUMS..."
( cd "$RELEASE_DIR" && sha256sum "$ASSET_NAME" install.sh > SHA256SUMS )

# ── Mock GitHub API responses ─────────────────────────────────────────────────
echo "==> Writing mock GitHub API JSON..."
cat > "$API_DIR/latest" <<EOF
{
  "tag_name": "${VERSION}",
  "name": "${VERSION}",
  "draft": false,
  "prerelease": true,
  "assets": [
    {"name": "${ASSET_NAME}"},
    {"name": "install.sh"},
    {"name": "SHA256SUMS"}
  ]
}
EOF
cp "$API_DIR/latest" "$API_DIR/tags/${VERSION}"

# ── Summary ───────────────────────────────────────────────────────────────────
cat <<EOF

==> Local release ready in ${DIST_ROOT}/

    ${DIST_ROOT}/release/${VERSION}/
        ${ASSET_NAME}
        install.sh           (SCRIPT_VERSION=local-${SHORT_SHA})
        SHA256SUMS

    ${DIST_ROOT}/api/repos/kolapsis/maintenant/releases/
        latest               (JSON)
        tags/${VERSION}      (JSON)

Next steps (run from the host):

  # Serve dist/ on the LAN
  cd ${DIST_ROOT} && python3 -m http.server 8000 --bind 0.0.0.0

  # From a Linux VM, replace HOST_IP with your host's LAN IP:
  HOST_IP=<host-IP>
  curl -fsSL http://\${HOST_IP}:8000/release/${VERSION}/install.sh -o /tmp/install.sh
  sudo MAINTENANT_VERSION=${VERSION} \\
       MAINTENANT_GITHUB_API=http://\${HOST_IP}:8000/api \\
       MAINTENANT_DOWNLOAD_BASE=http://\${HOST_IP}:8000/release \\
       sh /tmp/install.sh --skip-cosign

EOF
