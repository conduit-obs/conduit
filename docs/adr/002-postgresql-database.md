# ADR-002: PostgreSQL as Primary Database

## Status

Accepted

## Date

2026-01-15

## Context

Conduit needs a persistent data store for tenants, agents, configuration intents, fleets, rollouts, audit events, and API keys. The store must support multi-tenant isolation, JSONB for flexible schemas, and transactional consistency.

## Decision

We chose PostgreSQL as the primary relational database with Row-Level Security (RLS) for tenant isolation.

## Consequences

### Positive

- Mature, battle-tested RDBMS with excellent reliability
- Native JSONB support for flexible schemas (labels, capabilities, topology)
- Row-Level Security (RLS) provides database-enforced tenant isolation
- Rich indexing (GIN for JSONB, B-tree, partial indexes)
- Strong transactional guarantees (ACID)
- Excellent tooling and managed service availability (RDS, Cloud SQL, etc.)
- `@>` containment operator enables efficient label-selector matching

### Negative

- Requires careful connection pool management under high concurrency
- Schema migrations need coordination across deployments
- RLS adds query planning overhead

### Neutral

- pgx driver chosen for Go (high-performance, pure Go)
- Connection pooling via pgxpool

## Alternatives Considered

### Alternative 1: CockroachDB

Distributed SQL with horizontal scaling but adds operational complexity, and our scale requirements don't justify the trade-off at this stage.

### Alternative 2: MongoDB

Flexible schema but weaker transactional guarantees, no native RLS equivalent, and the relational model better fits our data access patterns (joins for fleet membership, rollout tracking).
