# Getting Started with Conduit

This guide takes you from zero to a working Conduit installation with active telemetry flow in under 30 minutes.

## Prerequisites

- **Docker** and **Docker Compose** (v2+)
- **curl** for API calls
- **Go 1.22+** (optional, for building from source)

## Quick Start

### 1. Clone and Start

```bash
git clone https://github.com/conduit-obs/conduit.git
cd conduit
docker compose up -d --build
```

Wait for services to be healthy:

```bash
# Check health
curl http://localhost:8080/healthz
# Expected: {"status":"ok"}

# Check version
curl http://localhost:8080/api/v1/version
```

Services available:
- **API**: http://localhost:8080
- **API Docs**: http://localhost:8080/api/v1/docs
- **PostgreSQL**: localhost:5432
- **Redis**: localhost:6379

### 2. Get a JWT Token

For local development, issue a token using the CLI:

```bash
# Build the CLI
go build -o bin/conduit ./cmd/conduit

# Issue a dev token (requires JWT keys — see auth setup)
bin/conduit auth issue-token --tenant-id <TENANT_ID> --roles admin
```

Or set a token in your environment:

```bash
export CONDUIT_TOKEN="your-jwt-token-here"
```

## Enroll Your First Agent

Register a collector agent with the control plane:

```bash
curl -X POST http://localhost:8080/api/v1/agents \
  -H "Authorization: Bearer $CONDUIT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "my-first-collector",
    "labels": {
      "env": "development",
      "region": "us-east-1"
    }
  }'
```

Verify the agent is registered:

```bash
curl http://localhost:8080/api/v1/agents \
  -H "Authorization: Bearer $CONDUIT_TOKEN"
```

## Create a Pipeline Intent

Create a config intent that defines a telemetry pipeline:

```bash
curl -X POST http://localhost:8080/api/v1/config/intents \
  -H "Authorization: Bearer $CONDUIT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "my-first-pipeline",
    "tags": ["getting-started"],
    "intent": {
      "version": "1.0",
      "pipelines": [{
        "name": "traces",
        "signal": "traces",
        "receivers": [{
          "type": "otlp",
          "protocol": "grpc",
          "endpoint": "0.0.0.0:4317"
        }],
        "processors": [{
          "type": "batch",
          "settings": {"send_batch_size": 1000}
        }],
        "exporters": [{
          "type": "otlp",
          "endpoint": "https://your-backend.example.com:4317"
        }]
      }]
    }
  }'
```

Validate an intent before creating:

```bash
curl -X POST http://localhost:8080/api/v1/config/validate \
  -H "Authorization: Bearer $CONDUIT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "version": "1.0",
    "pipelines": [{
      "name": "test",
      "signal": "traces",
      "receivers": [{"type": "otlp", "protocol": "grpc"}],
      "exporters": [{"type": "debug"}]
    }]
  }'
```

## Create a Fleet and Push Config via Rollout

### Create a Fleet

Group agents by label selector:

```bash
curl -X POST http://localhost:8080/api/v1/fleets \
  -H "Authorization: Bearer $CONDUIT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "dev-fleet",
    "selector": {"env": "development"}
  }'
```

### Promote the Intent

Only promoted intents can be used in rollouts:

```bash
curl -X POST http://localhost:8080/api/v1/config/intents/my-first-pipeline/promote \
  -H "Authorization: Bearer $CONDUIT_TOKEN"
```

### Create a Rollout

Push the promoted config to the fleet:

```bash
curl -X POST http://localhost:8080/api/v1/rollouts \
  -H "Authorization: Bearer $CONDUIT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "fleet_id": "<FLEET_ID>",
    "intent_id": "<INTENT_ID>",
    "strategy": "all-at-once"
  }'
```

Check rollout status:

```bash
curl http://localhost:8080/api/v1/rollouts/<ROLLOUT_ID> \
  -H "Authorization: Bearer $CONDUIT_TOKEN"
```

## Verify Telemetry Flow

### Check Agent Topology

```bash
curl http://localhost:8080/api/v1/topology \
  -H "Authorization: Bearer $CONDUIT_TOKEN"
```

### View Pipeline Templates

Conduit includes built-in templates for common patterns:

```bash
curl http://localhost:8080/api/v1/templates \
  -H "Authorization: Bearer $CONDUIT_TOKEN"
```

### Monitor System Metrics

```bash
curl http://localhost:8080/api/v1/metrics
```

### Browse API Documentation

Open http://localhost:8080/api/v1/docs in your browser for interactive API documentation.

## Using the CLI

The Conduit CLI provides shortcuts for common operations:

```bash
# Build
go build -o bin/conduit ./cmd/conduit

# List agents
bin/conduit agent list --token $CONDUIT_TOKEN

# Compile a config intent locally
bin/conduit config compile -f examples/basic-metrics.json

# Validate an intent
bin/conduit config validate -f examples/distributed-traces.json

# View topology
bin/conduit topology --token $CONDUIT_TOKEN

# List templates
bin/conduit template list --token $CONDUIT_TOKEN

# Show a template
bin/conduit template show otlp-ingestion --token $CONDUIT_TOKEN

# Check version
bin/conduit version
```

## Next Steps

- **Templates**: Browse built-in templates at `/api/v1/templates` and use them to create intents faster
- **Tags**: Organize intents with tags: `GET /api/v1/config/intents?tag=production`
- **Scheduled Rollouts**: Schedule config changes: `"scheduled_at": "2026-04-10T15:00:00Z"`
- **Canary Deployments**: Use `"strategy": "canary"` with `"canary_percent": 10`
- **Export/Import**: Move configs between environments with export/import endpoints
- **Policy Packs**: Combine templates into reusable compliance packs
- **API Keys**: Create scoped API keys for CI/CD: `POST /api/v1/api-keys`
- **Webhooks**: Get notified of events: `POST /api/v1/webhooks`

## Troubleshooting

### Services won't start

```bash
docker compose logs conduit
docker compose logs postgres
```

### Database connection errors

Ensure PostgreSQL is healthy:

```bash
docker compose exec postgres pg_isready -U conduit
```

### Authentication errors

Check your token is valid and has the right roles. For local dev:

```bash
bin/conduit auth issue-token --tenant-id <ID> --roles admin
```

### Port conflicts

Change ports in `.env` (copy from `.env.example`):

```bash
cp .env.example .env
# Edit API_PORT, POSTGRES_PORT, etc.
```
