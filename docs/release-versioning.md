# Release Versioning Strategy

## Semantic Versioning

Conduit follows [Semantic Versioning 2.0.0](https://semver.org/):

- **MAJOR** (X.0.0): Breaking API changes, incompatible schema migrations
- **MINOR** (0.X.0): New features, backward-compatible additions
- **PATCH** (0.0.X): Bug fixes, security patches, no functional changes

## Version Format

- Release tags: `vMAJOR.MINOR.PATCH` (e.g., `v1.0.0`, `v0.9.1`)
- Pre-release: `vMAJOR.MINOR.PATCH-PRERELEASE` (e.g., `v1.0.0-alpha.1`, `v1.0.0-beta.2`, `v1.0.0-rc.1`)
- Build metadata: `vMAJOR.MINOR.PATCH+BUILD` (e.g., `v1.0.0+20260409`)

## Pre-Release Conventions

| Stage | Format | Purpose |
|-------|--------|---------|
| Alpha | `v1.0.0-alpha.N` | Internal testing, APIs may change |
| Beta  | `v1.0.0-beta.N` | External testing, APIs stabilizing |
| RC    | `v1.0.0-rc.N` | Release candidate, final testing |

## Branch Strategy

- **main**: Latest development, always builds and passes tests
- **release/vX.Y**: Release branches for each minor version
- Tags on release branches trigger builds

## API Version Alignment

- API version (`/api/v1/`) aligns with MAJOR product version
- Minor/patch product updates don't change API version
- New API version (`/api/v2/`) only on MAJOR version bump

## Backwards Compatibility Guarantees

- **API**: Previous 2 minor versions supported
- **Database schema**: Forward migrations always provided, rollback for 1 minor version
- **CLI**: Backward-compatible within MAJOR version
- **Helm charts**: Values schema stable within MINOR version

## Deprecation Timeline

1. Feature marked as deprecated in release notes
2. Deprecation warning added to API responses and CLI output
3. Feature removed after 2 minor version cycles
4. Migration guide provided before removal

## Release Process

```bash
# 1. Ensure clean main branch
git checkout main && git pull

# 2. Run release script
./scripts/release.sh 1.0.0

# 3. Tag and push
git tag -a v1.0.0 -m "Release v1.0.0"
git push origin v1.0.0

# 4. GitHub Actions will:
#    - Build binaries (linux/darwin, amd64/arm64)
#    - Build and push Docker image
#    - Package Helm charts
#    - Create GitHub release with artifacts
```
