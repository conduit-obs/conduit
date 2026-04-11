# ADR-003: OpAMP as Agent Management Protocol

## Status

Accepted

## Date

2026-01-20

## Context

Conduit needs a bidirectional communication protocol between the control plane and collector agents for configuration delivery, health monitoring, and capability negotiation. The protocol must support mTLS authentication, efficient binary encoding, and graceful reconnection.

## Decision

We adopted OpAMP (Open Agent Management Protocol) as the standard protocol for agent-to-control-plane communication, implemented over gRPC with mTLS.

## Consequences

### Positive

- Industry standard protocol designed specifically for observability agent management
- Supports bidirectional streaming for real-time config push and status reporting
- Built-in capability negotiation and version management
- mTLS provides strong mutual authentication
- Efficient protobuf binary encoding
- Designed for large fleet management (10k+ agents)
- Active community development under OpenTelemetry project

### Negative

- Protocol is still evolving — may require updates as spec matures
- gRPC adds complexity compared to simple HTTP polling
- mTLS certificate management adds operational overhead

### Neutral

- Requires protobuf code generation toolchain
- Agent-side OpAMP client implementation needed for each collector type

## Alternatives Considered

### Alternative 1: HTTP Long-Polling

Simpler implementation but higher latency for config delivery, inefficient for large fleets, and no standard for capability negotiation.

### Alternative 2: MQTT

Good pub/sub model but not designed for the specific agent management lifecycle patterns we need (enrollment, capability reporting, config acknowledgment).
