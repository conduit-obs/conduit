#!/usr/bin/env bash
set -euo pipefail

# Conduit Agent Bootstrap Script
# Enrolls a collector agent with the Conduit control plane.
#
# Usage:
#   ./bootstrap.sh --control-plane https://conduit.example.com \
#                  --enrollment-token ABC123 \
#                  --collector-version latest \
#                  --fleet prod-us-east

VERSION="0.9.0"
CONTROL_PLANE=""
ENROLLMENT_TOKEN=""
COLLECTOR_VERSION="latest"
FLEET=""
DRY_RUN=false
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/conduit"
SERVICE_NAME="conduit-collector"
ROLLBACK_STEPS=()

usage() {
  cat <<EOF
Conduit Agent Bootstrap v${VERSION}

Usage: $0 [OPTIONS]

Options:
  --control-plane URL     Conduit control plane URL (required)
  --enrollment-token TOK  Enrollment token for registration (required)
  --collector-version VER Collector version to install (default: latest)
  --fleet NAME            Fleet to join on registration
  --install-dir DIR       Binary installation directory (default: /usr/local/bin)
  --dry-run               Show what would be done without making changes
  -h, --help              Show this help message

Examples:
  $0 --control-plane https://conduit.example.com --enrollment-token ABC123
  $0 --control-plane https://conduit.example.com --enrollment-token ABC123 --fleet prod --dry-run
EOF
  exit 0
}

log() { echo "[conduit-bootstrap] $*"; }
err() { echo "[conduit-bootstrap] ERROR: $*" >&2; }
warn() { echo "[conduit-bootstrap] WARN: $*" >&2; }

rollback() {
  err "Installation failed. Rolling back..."
  for step in "${ROLLBACK_STEPS[@]}"; do
    log "Rollback: $step"
    eval "$step" || warn "Rollback step failed: $step"
  done
  err "Rollback complete. Please check the errors above."
  exit 1
}

trap rollback ERR

# Parse arguments
while [[ $# -gt 0 ]]; do
  case "$1" in
    --control-plane) CONTROL_PLANE="$2"; shift 2 ;;
    --enrollment-token) ENROLLMENT_TOKEN="$2"; shift 2 ;;
    --collector-version) COLLECTOR_VERSION="$2"; shift 2 ;;
    --fleet) FLEET="$2"; shift 2 ;;
    --install-dir) INSTALL_DIR="$2"; shift 2 ;;
    --dry-run) DRY_RUN=true; shift ;;
    -h|--help) usage ;;
    *) err "Unknown option: $1"; usage ;;
  esac
done

# Validate required args
if [[ -z "$CONTROL_PLANE" ]]; then
  err "--control-plane is required"
  exit 1
fi
if [[ -z "$ENROLLMENT_TOKEN" ]]; then
  err "--enrollment-token is required"
  exit 1
fi

# Detect OS and architecture
detect_platform() {
  OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
  ARCH="$(uname -m)"

  case "$OS" in
    linux)  OS="linux" ;;
    darwin) OS="darwin" ;;
    *)      err "Unsupported OS: $OS"; exit 1 ;;
  esac

  case "$ARCH" in
    x86_64|amd64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *)             err "Unsupported architecture: $ARCH"; exit 1 ;;
  esac

  # Detect Linux distro
  DISTRO="unknown"
  if [[ "$OS" == "linux" ]]; then
    if [[ -f /etc/os-release ]]; then
      . /etc/os-release
      DISTRO="${ID:-unknown}"
    elif [[ -f /etc/redhat-release ]]; then
      DISTRO="rhel"
    fi
  fi

  log "Platform: ${OS}/${ARCH} (distro: ${DISTRO})"
}

# Check prerequisites
check_prerequisites() {
  local missing=()

  command -v curl >/dev/null 2>&1 || missing+=("curl")

  if [[ "$OS" == "linux" ]]; then
    command -v sha256sum >/dev/null 2>&1 || missing+=("sha256sum")
  else
    command -v shasum >/dev/null 2>&1 || missing+=("shasum")
  fi

  if [[ ${#missing[@]} -gt 0 ]]; then
    err "Missing required tools: ${missing[*]}"
    exit 1
  fi
}

# Verify SHA256 checksum
verify_checksum() {
  local file="$1"
  local expected="$2"

  local actual
  if command -v sha256sum >/dev/null 2>&1; then
    actual="$(sha256sum "$file" | awk '{print $1}')"
  else
    actual="$(shasum -a 256 "$file" | awk '{print $1}')"
  fi

  if [[ "$actual" != "$expected" ]]; then
    err "Checksum verification failed!"
    err "  Expected: $expected"
    err "  Got:      $actual"
    return 1
  fi
  log "Checksum verified: $actual"
}

# Download collector binary
download_collector() {
  local download_url="${CONTROL_PLANE}/downloads/collector/${COLLECTOR_VERSION}/${OS}/${ARCH}"
  local checksum_url="${download_url}.sha256"
  local tmp_dir
  tmp_dir="$(mktemp -d)"

  log "Downloading collector ${COLLECTOR_VERSION} for ${OS}/${ARCH}..."

  if $DRY_RUN; then
    log "[DRY-RUN] Would download from: $download_url"
    log "[DRY-RUN] Would verify checksum from: $checksum_url"
    log "[DRY-RUN] Would install to: ${INSTALL_DIR}/conduit-collector"
    return 0
  fi

  # Download binary
  if ! curl -fsSL -o "${tmp_dir}/conduit-collector" "$download_url" 2>/dev/null; then
    warn "Download from control plane failed, skipping binary download"
    warn "Install the collector binary manually to ${INSTALL_DIR}/conduit-collector"
    return 0
  fi

  # Download and verify checksum
  if curl -fsSL -o "${tmp_dir}/conduit-collector.sha256" "$checksum_url" 2>/dev/null; then
    local expected
    expected="$(cat "${tmp_dir}/conduit-collector.sha256" | awk '{print $1}')"
    verify_checksum "${tmp_dir}/conduit-collector" "$expected"
  else
    warn "Checksum file not available, skipping verification"
  fi

  # Install binary
  chmod +x "${tmp_dir}/conduit-collector"
  if [[ -w "$INSTALL_DIR" ]]; then
    mv "${tmp_dir}/conduit-collector" "${INSTALL_DIR}/conduit-collector"
  else
    sudo mv "${tmp_dir}/conduit-collector" "${INSTALL_DIR}/conduit-collector"
  fi
  ROLLBACK_STEPS+=("rm -f ${INSTALL_DIR}/conduit-collector")

  log "Collector installed to ${INSTALL_DIR}/conduit-collector"
  rm -rf "$tmp_dir"
}

# Create configuration directory and config file
create_config() {
  log "Creating configuration..."

  if $DRY_RUN; then
    log "[DRY-RUN] Would create config dir: $CONFIG_DIR"
    log "[DRY-RUN] Would write config to: ${CONFIG_DIR}/collector.yaml"
    return 0
  fi

  if [[ -w "$(dirname "$CONFIG_DIR")" ]]; then
    mkdir -p "$CONFIG_DIR"
  else
    sudo mkdir -p "$CONFIG_DIR"
  fi
  ROLLBACK_STEPS+=("rm -rf ${CONFIG_DIR}")

  local config_content
  config_content="$(cat <<YAML
# Conduit collector configuration
# Auto-generated by bootstrap.sh

control_plane:
  url: "${CONTROL_PLANE}"
  enrollment_token: "${ENROLLMENT_TOKEN}"
  fleet: "${FLEET}"

receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318

exporters:
  debug:
    verbosity: basic

extensions:
  health_check:
    endpoint: 0.0.0.0:13133

service:
  extensions: [health_check]
  pipelines:
    traces:
      receivers: [otlp]
      exporters: [debug]
    metrics:
      receivers: [otlp]
      exporters: [debug]
    logs:
      receivers: [otlp]
      exporters: [debug]
YAML
)"

  if [[ -w "$CONFIG_DIR" ]]; then
    echo "$config_content" > "${CONFIG_DIR}/collector.yaml"
  else
    echo "$config_content" | sudo tee "${CONFIG_DIR}/collector.yaml" > /dev/null
  fi

  log "Configuration written to ${CONFIG_DIR}/collector.yaml"
}

# Install systemd service (Linux only)
install_service() {
  if [[ "$OS" != "linux" ]]; then
    log "Skipping service installation on $OS (not Linux)"
    return 0
  fi

  if ! command -v systemctl >/dev/null 2>&1; then
    warn "systemd not found, skipping service installation"
    return 0
  fi

  log "Installing systemd service..."

  if $DRY_RUN; then
    log "[DRY-RUN] Would create systemd unit: /etc/systemd/system/${SERVICE_NAME}.service"
    log "[DRY-RUN] Would enable and start ${SERVICE_NAME}"
    return 0
  fi

  local unit_content
  unit_content="$(cat <<UNIT
[Unit]
Description=Conduit Collector Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=${INSTALL_DIR}/conduit-collector --config ${CONFIG_DIR}/collector.yaml
Restart=always
RestartSec=5
LimitNOFILE=65536
User=root

[Install]
WantedBy=multi-user.target
UNIT
)"

  echo "$unit_content" | sudo tee "/etc/systemd/system/${SERVICE_NAME}.service" > /dev/null
  ROLLBACK_STEPS+=("sudo rm -f /etc/systemd/system/${SERVICE_NAME}.service && sudo systemctl daemon-reload")

  sudo systemctl daemon-reload
  sudo systemctl enable "$SERVICE_NAME"
  sudo systemctl start "$SERVICE_NAME"

  log "Service ${SERVICE_NAME} installed and started"
}

# Register with control plane
register_agent() {
  local hostname
  hostname="$(hostname)"

  log "Registering agent '${hostname}' with control plane..."

  if $DRY_RUN; then
    log "[DRY-RUN] Would POST to ${CONTROL_PLANE}/api/v1/agents"
    log "[DRY-RUN] Agent name: ${hostname}, fleet: ${FLEET}"
    return 0
  fi

  local labels="{\"bootstrap\":\"true\"}"
  if [[ -n "$FLEET" ]]; then
    labels="{\"bootstrap\":\"true\",\"fleet\":\"${FLEET}\"}"
  fi

  local response
  response=$(curl -s -w "\n%{http_code}" \
    -X POST "${CONTROL_PLANE}/api/v1/agents" \
    -H "Authorization: Bearer ${ENROLLMENT_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "{\"name\":\"${hostname}\",\"labels\":${labels}}" 2>/dev/null) || true

  local status_code
  status_code=$(echo "$response" | tail -1)
  local body
  body=$(echo "$response" | head -n -1)

  if [[ "$status_code" == "201" ]] || [[ "$status_code" == "200" ]]; then
    log "Agent registered successfully"
  else
    warn "Agent registration returned status $status_code (may already be registered)"
  fi
}

# Health check
health_check() {
  log "Running health check..."

  if $DRY_RUN; then
    log "[DRY-RUN] Would check health at http://localhost:13133"
    log "[DRY-RUN] Would verify control plane connectivity"
    return 0
  fi

  # Check control plane connectivity
  if curl -sf "${CONTROL_PLANE}/healthz" >/dev/null 2>&1; then
    log "Control plane connectivity: OK"
  else
    warn "Control plane connectivity check failed"
  fi

  # Check local collector health (if running)
  if curl -sf "http://localhost:13133" >/dev/null 2>&1; then
    log "Collector health check: OK"
  else
    warn "Collector health check failed (may still be starting)"
  fi
}

# Main
main() {
  log "Conduit Agent Bootstrap v${VERSION}"
  log "==============================="

  if $DRY_RUN; then
    log "DRY-RUN MODE: No changes will be made"
    echo ""
  fi

  detect_platform
  check_prerequisites
  download_collector
  create_config
  install_service
  register_agent
  health_check

  echo ""
  if $DRY_RUN; then
    log "Dry-run complete. Run without --dry-run to apply changes."
  else
    log "Bootstrap complete!"
    log "  Control Plane: ${CONTROL_PLANE}"
    log "  Collector:     ${INSTALL_DIR}/conduit-collector"
    log "  Config:        ${CONFIG_DIR}/collector.yaml"
    if [[ "$OS" == "linux" ]] && command -v systemctl >/dev/null 2>&1; then
      log "  Service:       systemctl status ${SERVICE_NAME}"
    fi
  fi
}

main "$@"
