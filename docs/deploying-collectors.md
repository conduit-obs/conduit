# Deploying OTel Collectors with Conduit

This guide covers how to deploy OpenTelemetry Collectors that connect to your Conduit control plane for centralized configuration management.

## How It Works

```
┌─────────────┐         OpAMP/gRPC          ┌──────────────────┐
│  OTel        │ ◄────────────────────────► │  Conduit          │
│  Collector   │   config push/pull         │  Control Plane    │
│              │   heartbeat                │                    │
│  (your host) │   status reporting         │  (localhost:8080)  │
└─────────────┘                             └──────────────────┘
```

Conduit manages your collectors via the **OpAMP protocol** (Open Agent Management Protocol). Collectors register with the control plane, receive configuration pushes, report health, and can be organized into fleets.

## Prerequisites

- Conduit running (`docker compose up -d` or deployed)
- A JWT token or API key for authentication
- The Conduit control plane URL (e.g., `http://localhost:8080`)

## Option 1: Bootstrap Script (Linux/macOS)

The fastest way to enroll a collector on a single host.

### Step 1: Get an enrollment token

```bash
# Use your Conduit JWT token
export CONDUIT_TOKEN="your-jwt-token"

# Or create an API key via the UI (Settings > API Keys) or CLI:
curl -X POST http://localhost:8080/api/v1/api-keys \
  -H "Authorization: Bearer $CONDUIT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name": "collector-enrollment", "permissions": ["*"]}'
```

### Step 2: Create a fleet (if you haven't already)

```bash
curl -X POST http://localhost:8080/api/v1/fleets \
  -H "Authorization: Bearer $CONDUIT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name": "my-fleet", "selector": {"env": "production"}}'
```

### Step 3: Run the bootstrap script

```bash
curl -sSL https://your-conduit-server/bootstrap.sh | bash -s -- \
  --control-plane http://localhost:8080 \
  --enrollment-token "$CONDUIT_TOKEN" \
  --fleet my-fleet
```

Or download and run manually:

```bash
# Download
curl -o bootstrap.sh https://your-conduit-server/bootstrap.sh
chmod +x bootstrap.sh

# Dry run first
./bootstrap.sh \
  --control-plane http://localhost:8080 \
  --enrollment-token "$CONDUIT_TOKEN" \
  --fleet my-fleet \
  --dry-run

# Run for real
./bootstrap.sh \
  --control-plane http://localhost:8080 \
  --enrollment-token "$CONDUIT_TOKEN" \
  --fleet my-fleet
```

The bootstrap script will:
1. Detect your OS and architecture
2. Download the collector binary
3. Create config at `/etc/conduit/collector.yaml`
4. Install as a systemd service
5. Register with the control plane
6. Run a health check

### Step 4: Verify registration

```bash
curl http://localhost:8080/api/v1/agents \
  -H "Authorization: Bearer $CONDUIT_TOKEN" | jq .
```

You should see your collector in the agent list.

## Option 2: Docker

Run a collector as a Docker container pointing at your Conduit instance.

```bash
docker run -d \
  --name conduit-collector \
  -e CONDUIT_CONTROL_PLANE_URL=http://host.docker.internal:8080 \
  -e CONDUIT_ENROLLMENT_TOKEN="$CONDUIT_TOKEN" \
  -p 4317:4317 \
  -p 4318:4318 \
  -p 13133:13133 \
  otel/opentelemetry-collector-contrib:0.100.0 \
  --config /etc/otelcol/config.yaml
```

With a config file mounted:

```bash
docker run -d \
  --name conduit-collector \
  -v $(pwd)/collector-config.yaml:/etc/otelcol/config.yaml \
  -p 4317:4317 \
  -p 4318:4318 \
  otel/opentelemetry-collector-contrib:0.100.0
```

Then register the agent with Conduit:

```bash
curl -X POST http://localhost:8080/api/v1/agents \
  -H "Authorization: Bearer $CONDUIT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "docker-collector-01",
    "labels": {"env": "dev", "type": "docker"}
  }'
```

## Option 3: Kubernetes (Helm)

Deploy a fleet of collectors across your K8s cluster using the Conduit Collector Helm chart.

### Install the chart

```bash
helm install conduit-collectors ./backend/deploy/helm/conduit-collector \
  --set controlPlane.url=http://conduit-backend:8080 \
  --set controlPlane.token="$CONDUIT_TOKEN" \
  --set mode=DaemonSet
```

### Configuration options

| Value | Default | Description |
|-------|---------|-------------|
| `controlPlane.url` | `https://conduit.example.com` | Conduit control plane URL |
| `controlPlane.token` | `""` | Enrollment token |
| `controlPlane.tokenSecret` | `""` | K8s secret name containing token |
| `mode` | `DaemonSet` | `DaemonSet` (one per node) or `Deployment` (fixed replicas) |
| `replicaCount` | `3` | Replicas when mode=Deployment |
| `image.repository` | `otel/opentelemetry-collector-contrib` | Collector image |
| `image.tag` | `0.100.0` | Collector version |
| `resources.requests.cpu` | `100m` | CPU request |
| `resources.requests.memory` | `128Mi` | Memory request |

### DaemonSet mode (recommended for cluster monitoring)

Deploys one collector per node — ideal for collecting node metrics, container logs, and cluster events.

```bash
helm install conduit-collectors ./backend/deploy/helm/conduit-collector \
  --set controlPlane.url=http://conduit-backend:8080 \
  --set controlPlane.token="$CONDUIT_TOKEN" \
  --set mode=DaemonSet \
  --set resources.requests.cpu=200m \
  --set resources.requests.memory=256Mi
```

### Deployment mode (for specific workloads)

Deploys a fixed number of collector replicas — ideal for receiving OTLP from application pods.

```bash
helm install conduit-collectors ./backend/deploy/helm/conduit-collector \
  --set controlPlane.url=http://conduit-backend:8080 \
  --set controlPlane.token="$CONDUIT_TOKEN" \
  --set mode=Deployment \
  --set replicaCount=3
```

### Using a K8s secret for the token

```bash
# Create secret
kubectl create secret generic conduit-token --from-literal=token="$CONDUIT_TOKEN"

# Reference in Helm
helm install conduit-collectors ./backend/deploy/helm/conduit-collector \
  --set controlPlane.url=http://conduit-backend:8080 \
  --set controlPlane.tokenSecret=conduit-token
```

## Option 4: Register an Existing Collector

If you already have OTel Collectors running, you can register them with Conduit without replacing them.

### Register via API

```bash
curl -X POST http://localhost:8080/api/v1/agents \
  -H "Authorization: Bearer $CONDUIT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "existing-collector-prod-01",
    "labels": {
      "env": "production",
      "region": "us-east-1",
      "type": "existing"
    }
  }'
```

### Register via CLI

```bash
conduit agent list --token "$CONDUIT_TOKEN"
# The agent will appear after registration
```

## Pushing Configuration to Collectors

Once collectors are registered and organized into fleets, you can push pipeline configurations.

### 1. Create a pipeline intent

```bash
curl -X POST http://localhost:8080/api/v1/config/intents \
  -H "Authorization: Bearer $CONDUIT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "otlp-to-honeycomb",
    "tags": ["production"],
    "intent": {
      "version": "1.0",
      "pipelines": [{
        "name": "traces",
        "signal": "traces",
        "receivers": [{"type": "otlp", "protocol": "grpc", "endpoint": "0.0.0.0:4317"}],
        "processors": [{"type": "batch", "settings": {"send_batch_size": 1000}}],
        "exporters": [{"type": "otlp", "endpoint": "https://api.honeycomb.io:443", "headers": {"x-honeycomb-team": "YOUR_API_KEY"}}]
      }]
    }
  }'
```

### 2. Promote the intent

```bash
curl -X POST http://localhost:8080/api/v1/config/intents/otlp-to-honeycomb/promote \
  -H "Authorization: Bearer $CONDUIT_TOKEN"
```

### 3. Create a rollout

```bash
curl -X POST http://localhost:8080/api/v1/rollouts \
  -H "Authorization: Bearer $CONDUIT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "fleet_id": "YOUR_FLEET_ID",
    "intent_id": "YOUR_INTENT_ID",
    "strategy": "canary",
    "canary_percent": 10
  }'
```

### 4. Monitor the rollout

```bash
curl http://localhost:8080/api/v1/rollouts/YOUR_ROLLOUT_ID \
  -H "Authorization: Bearer $CONDUIT_TOKEN" | jq .
```

Or use the Conduit UI at http://localhost:3000/rollouts.

## Topology and Fleet Organization

Agents can report topology metadata (region, zone, cluster) for hierarchical fleet management.

```bash
# View topology tree
curl http://localhost:8080/api/v1/topology \
  -H "Authorization: Bearer $CONDUIT_TOKEN" | jq .

# Or via CLI
conduit topology --token "$CONDUIT_TOKEN"
```

## Troubleshooting

### Collector doesn't appear in Conduit

1. Check the collector can reach the control plane: `curl http://your-conduit-server:8080/healthz`
2. Verify the enrollment token is valid
3. Check collector logs for connection errors

### Configuration not being applied

1. Verify the intent is promoted: `GET /api/v1/config/intents`
2. Check the fleet selector matches the agent's labels
3. Check rollout status: `GET /api/v1/rollouts`

### Collector unhealthy

1. Check health endpoint: `curl http://collector-host:13133`
2. View agent health in Conduit: `GET /api/v1/agents?min_health=0`
3. Check for config drift: the Conduit dashboard shows drift indicators per agent
