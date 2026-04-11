# ADR-005: Helm Charts as Primary Deployment Model

## Status

Accepted

## Date

2026-02-01

## Context

Conduit needs a standardized, repeatable deployment mechanism that supports both SaaS and self-hosted installations. Kubernetes is the target platform for production deployments.

## Decision

We chose Helm charts as the primary deployment mechanism for Kubernetes environments, with Docker Compose for local development.

## Consequences

### Positive

- Helm is the de facto standard for Kubernetes application packaging
- Supports parameterized deployments via `values.yaml`
- Built-in upgrade and rollback capabilities
- Supports multiple environments (dev, staging, production) via value overrides
- Large ecosystem of shared charts for dependencies (PostgreSQL, Redis)
- Familiar to Kubernetes operators

### Negative

- Helm template syntax can be verbose and hard to debug
- Chart versioning adds maintenance burden
- Not suitable for non-Kubernetes deployments (Docker Compose used instead)

### Neutral

- Two charts maintained: `conduit` (control plane) and `conduit-collector` (fleet)
- Collector chart supports both DaemonSet and Deployment modes
- Docker Compose provides local development environment
- Terraform modules provide cloud infrastructure provisioning

## Alternatives Considered

### Alternative 1: Raw Kubernetes Manifests + Kustomize

More transparent but less parameterizable, no built-in upgrade lifecycle management, and less familiar to operators who expect Helm.

### Alternative 2: Operator Pattern

More powerful lifecycle management but significantly more code to maintain, and our deployment patterns don't require the complexity of a custom operator.
