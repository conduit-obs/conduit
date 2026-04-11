#!/usr/bin/env bash
set -euo pipefail

# Conduit Changelog Generator
# Parses conventional commits and generates a categorized CHANGELOG.

OUTPUT="${1:-CHANGELOG.md}"
SINCE="${2:-}"

log() { echo "[changelog] $*"; }

if [[ -n "$SINCE" ]]; then
  RANGE="${SINCE}..HEAD"
  log "Generating changelog since ${SINCE}"
else
  RANGE="HEAD"
  log "Generating changelog from all commits"
fi

# Collect commits by category
FEATURES=""
FIXES=""
BREAKING=""
DOCS=""
CHORE=""
OTHER=""

while IFS= read -r line; do
  [[ -z "$line" ]] && continue

  case "$line" in
    feat:*|feat\(*) FEATURES+="- ${line#feat*: }"$'\n' ;;
    fix:*|fix\(*)   FIXES+="- ${line#fix*: }"$'\n' ;;
    breaking:*|BREAKING*) BREAKING+="- ${line}"$'\n' ;;
    docs:*|docs\(*) DOCS+="- ${line#docs*: }"$'\n' ;;
    chore:*|chore\(*|ci:*|ci\(*|build:*) CHORE+="- ${line}"$'\n' ;;
    *)              OTHER+="- ${line}"$'\n' ;;
  esac
done < <(git log --pretty=format:"%s" "$RANGE" 2>/dev/null || echo "")

# Generate markdown
{
  echo "# Changelog"
  echo ""
  echo "## [Unreleased]"
  echo ""
  echo "Generated: $(date -u +%Y-%m-%d)"
  echo ""

  if [[ -n "$BREAKING" ]]; then
    echo "### Breaking Changes"
    echo ""
    echo "$BREAKING"
  fi

  if [[ -n "$FEATURES" ]]; then
    echo "### Features"
    echo ""
    echo "$FEATURES"
  fi

  if [[ -n "$FIXES" ]]; then
    echo "### Bug Fixes"
    echo ""
    echo "$FIXES"
  fi

  if [[ -n "$DOCS" ]]; then
    echo "### Documentation"
    echo ""
    echo "$DOCS"
  fi

  if [[ -n "$CHORE" ]]; then
    echo "### Maintenance"
    echo ""
    echo "$CHORE"
  fi

  if [[ -n "$OTHER" ]]; then
    echo "### Other"
    echo ""
    echo "$OTHER"
  fi
} > "$OUTPUT"

log "Changelog written to ${OUTPUT}"
