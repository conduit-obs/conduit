# ADR-001: Go as Primary Implementation Language

## Status

Accepted

## Date

2026-01-15

## Context

Conduit is a control plane for managing OpenTelemetry collector fleets. It needs to be performant, easily deployable as a single binary, and align with the ecosystem it serves. The OpenTelemetry Collector itself is written in Go, as is much of the cloud-native tooling ecosystem (Kubernetes, Prometheus, etc.).

## Decision

We chose Go as the primary implementation language for the Conduit control plane, CLI, and agent management components.

## Consequences

### Positive

- Single static binary deployment — no runtime dependencies
- Excellent concurrency model (goroutines) for managing many agent connections
- Strong standard library for HTTP servers, JSON, crypto
- Ecosystem alignment with OpenTelemetry, Kubernetes, and cloud-native tooling
- Fast compilation and testing cycles
- Built-in cross-compilation for multi-platform support
- Strong typing catches errors at compile time

### Negative

- Less expressive than languages like Rust or Python for complex data transformations
- Verbose error handling patterns
- Limited generics (improving in recent versions)

### Neutral

- Team needs Go proficiency (common in cloud-native engineering)
- gRPC/protobuf toolchain required for OpAMP protocol

## Alternatives Considered

### Alternative 1: Rust

Excellent performance and memory safety but steeper learning curve, slower iteration speed, and smaller pool of contributors in the observability space.

### Alternative 2: TypeScript/Node.js

Good developer experience but weaker concurrency model for managing thousands of agent connections, and runtime dependency adds deployment complexity.
