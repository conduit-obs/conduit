#!/usr/bin/env bash
set -euo pipefail

# Conduit Release Artifact Signing
# Generates checksums, SBOM, and (optionally) cosign signatures.

ARTIFACTS_DIR="${1:-bin}"

log() { echo "[sign-release] $*"; }

log "Signing release artifacts in ${ARTIFACTS_DIR}/"

# Generate SHA256 checksums
log "Generating SHA256 checksums..."
cd "$ARTIFACTS_DIR"
find . -type f ! -name 'SHA256SUMS' ! -name 'checksums.txt' ! -name '*.sig' ! -name 'sbom.json' \
  -exec sha256sum {} \; > checksums.txt
log "  Written: checksums.txt"
cd - >/dev/null

# Generate SBOM stub
log "Generating SBOM..."
SBOM_FILE="${ARTIFACTS_DIR}/sbom.json"
cat > "$SBOM_FILE" <<EOF
{
  "bomFormat": "CycloneDX",
  "specVersion": "1.5",
  "version": 1,
  "metadata": {
    "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
    "tools": [{"vendor": "Conduit", "name": "sign-release.sh"}],
    "component": {
      "type": "application",
      "name": "conduit",
      "bom-ref": "conduit"
    }
  },
  "components": [
$(go version -m "${ARTIFACTS_DIR}/"conduit-* 2>/dev/null | grep -E "^\s+dep" | awk '{printf "    {\"type\": \"library\", \"name\": \"%s\", \"version\": \"%s\"},\n", $2, $3}' | sed '$ s/,$//')
  ]
}
EOF
log "  Written: sbom.json"

# Cosign signing (requires cosign installed and COSIGN_KEY set)
# Uncomment the following to enable cosign signing:
#
# if command -v cosign >/dev/null 2>&1; then
#   log "Signing with cosign..."
#   for f in "${ARTIFACTS_DIR}/"conduit-*; do
#     [[ -f "$f" ]] || continue
#     cosign sign-blob --key "${COSIGN_KEY}" --output-signature "${f}.sig" "$f"
#     log "  Signed: $(basename $f)"
#   done
# else
#   log "cosign not found — skipping signature generation"
#   log "Install: https://docs.sigstore.dev/cosign/installation/"
# fi

log ""
log "Signing complete!"
log "  Checksums: ${ARTIFACTS_DIR}/checksums.txt"
log "  SBOM:      ${ARTIFACTS_DIR}/sbom.json"
