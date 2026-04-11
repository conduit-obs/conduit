# ADR-004: REST API with JSON for External Interfaces

## Status

Accepted

## Date

2026-01-25

## Context

Conduit exposes APIs for the Web UI, CLI, SDKs, and third-party integrations. The API must be intuitive, well-documented, and accessible from any language or tool.

## Decision

We chose RESTful HTTP APIs with JSON payloads for all external-facing interfaces, using Go's standard `net/http` ServeMux with pattern-based routing.

## Consequences

### Positive

- Universal compatibility — any HTTP client can interact with Conduit
- JSON is the most widely supported data format across languages
- Easy to document with OpenAPI/Swagger specifications
- Curl-friendly for debugging and quick scripting
- Standard HTTP semantics (GET/POST/PATCH/DELETE) are well understood
- Go's `net/http` ServeMux (Go 1.22+) provides pattern matching without external dependencies

### Negative

- REST lacks the strong typing of gRPC for internal service communication
- JSON parsing overhead compared to binary protocols
- No built-in streaming (WebSocket used separately for events)

### Neutral

- API versioning via URL path (`/api/v1/`)
- Authentication via Bearer JWT and X-API-Key headers
- WebSocket used for real-time event streaming at `/api/v1/events/stream`

## Alternatives Considered

### Alternative 1: gRPC for All APIs

Strong typing and streaming but poor browser support, requires protobuf toolchain for every client, and harder to debug with standard tools.

### Alternative 2: GraphQL

Flexible queries but adds complexity, harder to cache, and our resource-oriented API fits REST patterns naturally.
