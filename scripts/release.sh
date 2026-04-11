#!/usr/bin/env bash
set -euo pipefail

# Conduit Release Script
# Validates version, runs tests, builds artifacts, creates git tag.

VERSION="${1:-}"

usage() {
  echo "Usage: $0 <version>"
  echo "  version: Semantic version (e.g., 0.9.0, 1.0.0-beta.1)"
  echo ""
  echo "Example: $0 1.0.0"
  exit 1
}

log() { echo "[release] $*"; }
err() { echo "[release] ERROR: $*" >&2; exit 1; }

[[ -z "$VERSION" ]] && usage

# Validate semver format
if ! echo "$VERSION" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.]+)?$'; then
  err "Invalid version format: $VERSION (expected MAJOR.MINOR.PATCH[-prerelease])"
fi

TAG="v${VERSION}"
log "Preparing release ${TAG}"

# Check clean working tree
if [[ -n "$(git status --porcelain)" ]]; then
  err "Working tree is not clean. Commit or stash changes first."
fi

# Check we're on main or release branch
BRANCH="$(git rev-parse --abbrev-ref HEAD)"
if [[ "$BRANCH" != "main" ]] && [[ ! "$BRANCH" =~ ^release/ ]]; then
  err "Releases must be from 'main' or 'release/*' branch (current: $BRANCH)"
fi

# Run tests
log "Running tests..."
go test ./... -count=1 || err "Tests failed"

# Build binaries
log "Building binaries..."
LDFLAGS="-X github.com/conduit-obs/conduit/internal/version.Version=${VERSION} -X github.com/conduit-obs/conduit/internal/version.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ) -X github.com/conduit-obs/conduit/internal/version.GitCommit=$(git rev-parse --short HEAD)"

for platform in "linux/amd64" "linux/arm64" "darwin/amd64" "darwin/arm64"; do
  IFS='/' read -r goos goarch <<< "$platform"
  output="bin/conduit-${goos}-${goarch}"
  log "  Building ${output}..."
  GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 go build -ldflags "$LDFLAGS" -o "$output" ./cmd/conduit
done

# Generate checksums
log "Generating checksums..."
cd bin && sha256sum conduit-* > SHA256SUMS && cd ..

log ""
log "Release ${TAG} is ready!"
log "  Binaries:   bin/"
log "  Checksums:  bin/SHA256SUMS"
log ""
log "To create the tag and push:"
log "  git tag -a ${TAG} -m 'Release ${TAG}'"
log "  git push origin ${TAG}"
