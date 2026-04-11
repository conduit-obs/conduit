# Compatibility Matrix

## Supported Versions

| Component | Current | Previous | EOL |
|-----------|---------|----------|-----|
| Control Plane | 0.9.x | - | - |
| Collector Agent | 0.100.x | 0.99.x | 0.98.x |
| CLI | 0.9.x | - | - |
| Go SDK | 0.9.x | - | - |

## Control Plane x Collector Compatibility

| Control Plane | Collector 0.100.x | Collector 0.99.x | Collector 0.98.x |
|---------------|-------------------|-------------------|-------------------|
| 0.9.x | Full | Full | Limited |

**Full**: All features supported
**Limited**: Core features work, some new features unavailable

## API Versions

| API Version | Status | Introduced | Sunset |
|-------------|--------|------------|--------|
| v1 | Current | 0.1.0 | - |

## Platform Support

| Platform | Architecture | Status |
|----------|-------------|--------|
| Linux | amd64 | Supported |
| Linux | arm64 | Supported |
| macOS | amd64 | Supported |
| macOS | arm64 | Supported |
| Windows | amd64 | Community |

## Kubernetes Compatibility

| Kubernetes Version | Status |
|-------------------|--------|
| 1.30.x | Supported |
| 1.29.x | Supported |
| 1.28.x | Supported |
| < 1.28 | Unsupported |

## Database Compatibility

| PostgreSQL Version | Status |
|-------------------|--------|
| 16.x | Supported (recommended) |
| 15.x | Supported |
| 14.x | Limited |

## Testing

Compatibility is validated automatically via:
- `tests/compat/compat_test.go` — API endpoint matrix testing
- CI workflows test against supported PostgreSQL versions
- Helm chart validation via `helm lint`
