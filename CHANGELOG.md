# Changelog

All notable changes to Conduit will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Features

- Pipeline template system with 6 built-in templates (otlp-ingestion, k8s-cluster-telemetry, redact-pii, drop-sensitive-attrs, trace-sampling, log-routing)
- Policy pack composition framework
- Feature flag system with per-tenant overrides and percentage rollout
- OpenAPI 3.0 specification with Swagger UI at /api/v1/docs
- Go SDK client library
- Comprehensive CI/CD pipeline with GitHub Actions
- Performance and load testing framework
- Compatibility testing matrix

### Infrastructure

- Helm charts for control plane and collector fleet
- Terraform modules for AWS (EKS + RDS)
- Bootstrap script for agent enrollment
- Air-gapped installation bundle support
- Docker Compose local development environment with Redis and mock collectors

### Documentation

- Getting Started guide
- Architecture Decision Records (ADR-001 through ADR-005)
- Release versioning strategy
- Compatibility matrix
- 10+ example pipeline configurations
