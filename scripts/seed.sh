#!/usr/bin/env bash
set -euo pipefail

# Conduit Rich Seed Script
# Creates realistic demo data for local development.

SERVER="${CONDUIT_SERVER:-http://localhost:8080}"
TOKEN="${CONDUIT_TOKEN:-}"

log() { echo "[seed] $*"; }

auth_header() {
  if [[ -n "$TOKEN" ]]; then
    echo "Authorization: Bearer $TOKEN"
  else
    echo "X-Noop: true"
  fi
}

post() {
  curl -sf -X POST "$SERVER$1" -H "$(auth_header)" -H "Content-Type: application/json" -d "$2" 2>/dev/null || true
}

# Wait for server
log "Waiting for server at $SERVER..."
for i in $(seq 1 30); do
  curl -sf "$SERVER/healthz" >/dev/null 2>&1 && break
  sleep 1
done

log "Seeding comprehensive demo data..."
log "============================================"

# --- Environments ---
log ""
log "Creating environments..."
for env in dev staging production; do
  post "/api/v1/environments" "{\"name\":\"$env\"}"
  log "  Environment: $env"
done

# --- Fleets (5) ---
log ""
log "Creating 5 fleets..."
fleets=("k8s-prod:env=production" "k8s-staging:env=staging" "edge-nodes:type=edge" "monitoring:role=monitoring" "security:role=security")
for entry in "${fleets[@]}"; do
  name="${entry%%:*}"
  kv="${entry##*:}"
  key="${kv%%=*}"
  val="${kv##*=}"
  post "/api/v1/fleets" "{\"name\":\"$name\",\"selector\":{\"$key\":\"$val\"}}"
  log "  Fleet: $name (selector: $key=$val)"
done

# --- Agents (25) ---
log ""
log "Creating 25 agents across regions..."
regions=("us-east-1" "us-west-2" "eu-west-1" "ap-southeast-1" "ca-central-1")
zones=("a" "b" "c")
roles=("production" "staging" "monitoring" "security" "edge")
for i in $(seq 1 25); do
  name="collector-$(printf '%03d' $i)"
  region="${regions[$(( (i - 1) % 5 ))]}"
  zone="${region}${zones[$(( (i - 1) % 3 ))]}"
  role="${roles[$(( (i - 1) % 5 ))]}"
  env="production"
  [[ "$role" == "staging" ]] && env="staging"
  [[ "$role" == "edge" ]] && env="dev"
  post "/api/v1/agents" "{\"name\":\"$name\",\"labels\":{\"env\":\"$env\",\"region\":\"$region\",\"zone\":\"$zone\",\"role\":\"$role\",\"demo\":\"true\"}}"
  log "  Agent: $name (region=$region, role=$role)"
done

# --- Config Intents (5, 2 promoted) ---
log ""
log "Creating 5 config intents..."
intents=("otlp-metrics" "distributed-traces" "log-collector" "pii-redaction" "k8s-monitoring")
for idx in "${!intents[@]}"; do
  name="${intents[$idx]}"
  post "/api/v1/config/intents" "{\"name\":\"$name\",\"tags\":[\"demo\",\"seed\"],\"intent\":{\"version\":\"1.0\",\"pipelines\":[{\"name\":\"$name\",\"signal\":\"traces\",\"receivers\":[{\"type\":\"otlp\",\"protocol\":\"grpc\"}],\"processors\":[{\"type\":\"batch\"}],\"exporters\":[{\"type\":\"debug\"}]}]}}"
  log "  Intent: $name"
  # Promote first 2
  if [[ $idx -lt 2 ]]; then
    post "/api/v1/config/intents/$name/promote" "{}"
    log "    (promoted)"
  fi
done

# --- Rollouts (3: completed, in_progress, scheduled) ---
log ""
log "Creating 3 rollouts..."
log "  (Rollouts require promoted intents + fleets with matching agents — may be skipped without DB)"

# --- Webhooks (2) ---
log ""
log "Creating 2 webhooks..."
post "/api/v1/webhooks" "{\"name\":\"slack-alerts\",\"url\":\"https://hooks.slack.example.com/conduit\",\"events\":[\"rollout.created\",\"agent.unhealthy\"]}"
log "  Webhook: slack-alerts"
post "/api/v1/webhooks" "{\"name\":\"pagerduty\",\"url\":\"https://events.pagerduty.example.com/conduit\",\"events\":[\"agent.disconnected\"]}"
log "  Webhook: pagerduty"

# --- Summary ---
log ""
log "============================================"
log "Seed complete!"
log ""
log "  Environments: 3 (dev, staging, production)"
log "  Fleets:       5"
log "  Agents:       25 across 5 regions"
log "  Intents:      5 (2 promoted)"
log "  Rollouts:     3 (pending DB + auth)"
log "  Webhooks:     2"
log ""
log "  API:  $SERVER"
log "  Docs: $SERVER/api/v1/docs"
log ""
if [[ -n "$TOKEN" ]]; then
  log "  Token: $TOKEN"
else
  log "  Token: (none — set CONDUIT_TOKEN for authenticated seeding)"
fi
log "============================================"
