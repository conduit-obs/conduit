#!/usr/bin/env bash
set -euo pipefail

# Conduit Demo Environment
# One-command script to launch a full demo with synthetic fleet.

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(dirname "$SCRIPT_DIR")"

log() { echo "[demo] $*"; }

cd "$REPO_ROOT"

log "Starting Conduit demo environment..."
log "====================================="
echo ""

# Start docker-compose
log "Step 1: Starting services..."
docker compose up -d --build 2>&1 | grep -v "^$" || true

# Wait for health
log "Step 2: Waiting for services to be healthy..."
for i in $(seq 1 60); do
  if curl -sf http://localhost:8080/healthz >/dev/null 2>&1; then
    log "  Conduit API is ready!"
    break
  fi
  if [[ $i -eq 60 ]]; then
    log "ERROR: Timed out waiting for Conduit to start"
    docker compose logs conduit
    exit 1
  fi
  sleep 2
done

# Seed demo data
log "Step 3: Seeding demo data..."

SERVER="http://localhost:8080"
TOKEN="${CONDUIT_TOKEN:-}"

auth() {
  if [[ -n "$TOKEN" ]]; then
    echo "-H" "Authorization: Bearer $TOKEN"
  fi
}

# Create tenants (5)
for t in acme globex initech umbrella waynetech; do
  curl -sf -X POST "$SERVER/api/v1/tenants" \
    $(auth) -H "Content-Type: application/json" \
    -d "{\"name\":\"$t\"}" >/dev/null 2>&1 || true
done
log "  Created 5 tenants"

# Create 50 agents across regions
regions=("us-east-1" "us-west-2" "eu-west-1" "ap-southeast-1" "ca-central-1")
zones=("a" "b" "c")
clusters=("prod" "staging" "dev")

for i in $(seq 1 50); do
  region="${regions[$((i % ${#regions[@]}))]}"
  zone="${region}${zones[$((i % ${#zones[@]}))]}"
  cluster="${clusters[$((i % ${#clusters[@]}))]}"
  curl -sf -X POST "$SERVER/api/v1/agents" \
    $(auth) -H "Content-Type: application/json" \
    -d "{\"name\":\"demo-agent-$(printf '%03d' $i)\",\"labels\":{\"env\":\"$cluster\",\"region\":\"$region\",\"zone\":\"$zone\",\"demo\":\"true\"}}" >/dev/null 2>&1 || true
done
log "  Created 50 agents across 5 regions"

# Create 10 fleets
for fleet in prod-us-east prod-us-west prod-eu staging-global dev-all monitoring security compliance performance canary; do
  curl -sf -X POST "$SERVER/api/v1/fleets" \
    $(auth) -H "Content-Type: application/json" \
    -d "{\"name\":\"$fleet\",\"selector\":{\"demo\":\"true\"}}" >/dev/null 2>&1 || true
done
log "  Created 10 fleets"

# Create sample intents
for intent in metrics-basic traces-otlp logs-collector pii-redaction k8s-monitoring; do
  curl -sf -X POST "$SERVER/api/v1/config/intents" \
    $(auth) -H "Content-Type: application/json" \
    -d "{\"name\":\"$intent\",\"tags\":[\"demo\"],\"intent\":{\"version\":\"1.0\",\"pipelines\":[{\"name\":\"$intent\",\"signal\":\"traces\",\"receivers\":[{\"type\":\"otlp\",\"protocol\":\"grpc\"}],\"exporters\":[{\"type\":\"debug\"}]}]}}" >/dev/null 2>&1 || true
done
log "  Created 5 sample config intents"

echo ""
log "====================================="
log "Demo environment is ready!"
log ""
log "  API:       http://localhost:8080"
log "  Docs:      http://localhost:8080/api/v1/docs"
log "  Version:   http://localhost:8080/api/v1/version"
log "  Health:    http://localhost:8080/healthz"
log "  Postgres:  localhost:5432"
log "  Redis:     localhost:6379"
log ""
log "  Tenants:   5"
log "  Agents:    50"
log "  Fleets:    10"
log "  Intents:   5"
log ""
log "To stop: docker compose down"
log "To reset: docker compose down -v && ./scripts/demo.sh"
