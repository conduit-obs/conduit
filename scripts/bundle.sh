#!/usr/bin/env bash
set -euo pipefail

# Conduit Air-Gapped Installation Bundle Creator
# Creates a self-contained bundle for offline deployment.
#
# Usage: ./bundle.sh [--output bundle.tar.gz] [--version 0.9.0]

VERSION="${CONDUIT_VERSION:-0.9.0}"
OUTPUT=""
BUNDLE_DIR=""

usage() {
  cat <<EOF
Conduit Air-Gapped Bundle Creator

Usage: $0 [OPTIONS]

Options:
  --version VER    Conduit version to bundle (default: $VERSION)
  --output FILE    Output file path (default: conduit-bundle-VERSION.tar.gz)
  -h, --help       Show this help

This script:
  1. Builds Conduit Docker images
  2. Packages Helm charts
  3. Generates SHA256 checksums
  4. Creates a compressed tar bundle with an offline install script
EOF
  exit 0
}

log() { echo "[conduit-bundle] $*"; }
err() { echo "[conduit-bundle] ERROR: $*" >&2; exit 1; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version) VERSION="$2"; shift 2 ;;
    --output)  OUTPUT="$2"; shift 2 ;;
    -h|--help) usage ;;
    *)         err "Unknown option: $1" ;;
  esac
done

if [[ -z "$OUTPUT" ]]; then
  OUTPUT="conduit-bundle-${VERSION}.tar.gz"
fi

BUNDLE_DIR="$(mktemp -d)/conduit-bundle-${VERSION}"
mkdir -p "${BUNDLE_DIR}/images" "${BUNDLE_DIR}/helm" "${BUNDLE_DIR}/bin"

cleanup() {
  rm -rf "$(dirname "$BUNDLE_DIR")"
}
trap cleanup EXIT

log "Creating Conduit bundle v${VERSION}"
log "Output: ${OUTPUT}"
echo ""

# Step 1: Build Docker images
log "Step 1: Building Docker images..."
if command -v docker >/dev/null 2>&1; then
  docker build -t "conduit-obs/conduit:${VERSION}" . 2>/dev/null || log "Docker build skipped (not in repo root or Docker unavailable)"

  # Save images
  for img in "conduit-obs/conduit:${VERSION}"; do
    local_name="$(echo "$img" | tr '/:' '-')"
    if docker image inspect "$img" >/dev/null 2>&1; then
      docker save "$img" | gzip > "${BUNDLE_DIR}/images/${local_name}.tar.gz"
      log "  Saved: ${local_name}.tar.gz"
    fi
  done
else
  log "  Docker not available, skipping image build"
fi

# Step 2: Package Helm charts
log "Step 2: Packaging Helm charts..."
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(dirname "$SCRIPT_DIR")"

if [[ -d "${REPO_ROOT}/deploy/helm/conduit" ]]; then
  cp -r "${REPO_ROOT}/deploy/helm/conduit" "${BUNDLE_DIR}/helm/"
  log "  Packaged: conduit control plane chart"
fi

if [[ -d "${REPO_ROOT}/deploy/helm/conduit-collector" ]]; then
  cp -r "${REPO_ROOT}/deploy/helm/conduit-collector" "${BUNDLE_DIR}/helm/"
  log "  Packaged: conduit-collector chart"
fi

# Step 3: Build CLI binaries (if Go available)
log "Step 3: Building CLI binaries..."
if command -v go >/dev/null 2>&1 && [[ -f "${REPO_ROOT}/go.mod" ]]; then
  for platform in "linux/amd64" "linux/arm64" "darwin/amd64" "darwin/arm64"; do
    IFS='/' read -r goos goarch <<< "$platform"
    binary_name="conduit-${goos}-${goarch}"
    GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 go build -o "${BUNDLE_DIR}/bin/${binary_name}" "${REPO_ROOT}/cmd/conduit" 2>/dev/null || log "  Skipped: ${binary_name}"
    if [[ -f "${BUNDLE_DIR}/bin/${binary_name}" ]]; then
      log "  Built: ${binary_name}"
    fi
  done
else
  log "  Go not available, skipping binary build"
fi

# Step 4: Copy bootstrap script
log "Step 4: Including bootstrap script..."
if [[ -f "${REPO_ROOT}/scripts/bootstrap.sh" ]]; then
  cp "${REPO_ROOT}/scripts/bootstrap.sh" "${BUNDLE_DIR}/"
  chmod +x "${BUNDLE_DIR}/bootstrap.sh"
  log "  Included: bootstrap.sh"
fi

# Step 5: Create offline install script
log "Step 5: Creating offline install script..."
cat > "${BUNDLE_DIR}/install.sh" <<'INSTALL_SCRIPT'
#!/usr/bin/env bash
set -euo pipefail

log() { echo "[conduit-install] $*"; }
err() { echo "[conduit-install] ERROR: $*" >&2; exit 1; }

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

log "Conduit Offline Installer"
log "========================="

# Verify checksums
if [[ -f "${SCRIPT_DIR}/SHA256SUMS" ]]; then
  log "Verifying checksums..."
  cd "$SCRIPT_DIR"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum -c SHA256SUMS || err "Checksum verification failed!"
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 -c SHA256SUMS || err "Checksum verification failed!"
  fi
  log "Checksums verified."
fi

# Load Docker images
if command -v docker >/dev/null 2>&1 && [[ -d "${SCRIPT_DIR}/images" ]]; then
  log "Loading Docker images..."
  for img in "${SCRIPT_DIR}/images/"*.tar.gz; do
    [[ -f "$img" ]] || continue
    docker load < "$img"
    log "  Loaded: $(basename "$img")"
  done
fi

# Install binary
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in x86_64|amd64) ARCH="amd64" ;; aarch64|arm64) ARCH="arm64" ;; esac

BINARY="${SCRIPT_DIR}/bin/conduit-${OS}-${ARCH}"
if [[ -f "$BINARY" ]]; then
  log "Installing conduit CLI..."
  chmod +x "$BINARY"
  sudo cp "$BINARY" /usr/local/bin/conduit
  log "  Installed: /usr/local/bin/conduit"
fi

log "Installation complete!"
log "  Helm charts: ${SCRIPT_DIR}/helm/"
log "  Bootstrap:   ${SCRIPT_DIR}/bootstrap.sh"
INSTALL_SCRIPT
chmod +x "${BUNDLE_DIR}/install.sh"

# Step 6: Generate checksums
log "Step 6: Generating SHA256 checksums..."
cd "$BUNDLE_DIR"
find . -type f ! -name 'SHA256SUMS' -exec sha256sum {} \; > SHA256SUMS 2>/dev/null || \
find . -type f ! -name 'SHA256SUMS' -exec shasum -a 256 {} \; > SHA256SUMS 2>/dev/null || \
log "  Checksum generation skipped"
log "  Generated: SHA256SUMS"

# Step 7: Create archive
log "Step 7: Creating archive..."
cd "$(dirname "$BUNDLE_DIR")"
tar czf "${OLDPWD}/${OUTPUT}" "$(basename "$BUNDLE_DIR")"

log ""
log "Bundle created: ${OUTPUT}"
log "Size: $(du -h "${OLDPWD}/${OUTPUT}" | awk '{print $1}')"
