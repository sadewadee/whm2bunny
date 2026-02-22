# CI/CD Documentation

## Overview

whm2bunny uses GitHub Actions for continuous integration and continuous deployment. The CI/CD pipeline ensures code quality, security, and automated releases.

## Workflows

### 1. Test Pipeline (`.github/workflows/test.yml`)

**Triggers:** Push to main/develop, Pull Requests to main

**Jobs:**
| Job | Description |
|-----|-------------|
| `test` | Runs tests on Go 1.21, 1.22, 1.23 with race detection |
| `lint` | Runs golangci-lint with 20+ linters |
| `security` | Runs gosec and govulncheck for security scanning |
| `build` | Builds and verifies the binary |

**Coverage:** Automatically uploaded to Codecov

### 2. Release Pipeline (`.github/workflows/release.yml`)

**Triggers:** Git tags (v*)

**Jobs:**
| Job | Description |
|-----|-------------|
| `build` | Builds binaries for Linux/macOS (amd64/arm64) |
| `release` | Creates GitHub Release with changelog |
| `docker` | Builds and pushes Docker multi-arch images |

**Artifacts:**
- `whm2bunny-linux-amd64.tar.gz`
- `whm2bunny-linux-arm64.tar.gz`
- `whm2bunny-darwin-amd64.tar.gz`
- `whm2bunny-darwin-arm64.tar.gz`
- `checksums.sha256`

### 3. Dependency Review (`.github/workflows/dependency-review.yml`)

**Triggers:** Pull Requests to main

**Jobs:**
| Job | Description |
|-----|-------------|
| `dependency-review` | Checks for vulnerable dependencies |
| `dependency-graph` | Generates dependency graph |

## Local CI Simulation

Run CI checks locally before pushing:

```bash
# Run all CI checks
make ci

# Or step by step
make fmt        # Format code
make vet        # Run go vet
make lint       # Run golangci-lint
make test       # Run tests with coverage
make build      # Build binary
```

## Required Secrets

Configure these secrets in GitHub repository settings:

| Secret | Required For | Description |
|--------|--------------|-------------|
| `DOCKER_USERNAME` | Docker push | Docker Hub username |
| `DOCKER_PASSWORD` | Docker push | Docker Hub access token |
| `CODECOV_TOKEN` | Coverage upload | Codecov upload token (optional) |

## Branch Protection

Recommended branch protection rules for `main`:

1. **Required Checks:**
   - test (Go 1.22)
   - lint
   - security
   - build

2. **Require PR Reviews:**
   - 1 approval required
   - Dismiss stale reviews on new commits

3. **Additional Settings:**
   - Require linear history
   - Include administrators
   - Restrict force pushes

## Security Scanning

### Gosec

Scans for common security issues:
- SQL injection
- Hardcoded credentials
- Insecure TLS configuration
- File path injection

### Govulncheck

Checks for known vulnerabilities in dependencies:
```bash
# Run locally
go install golang.org/x/vuln/cmd/govulncheck@latest
govulncheck ./...
```

## Linting

The project uses golangci-lint with comprehensive rules:

```bash
# Run locally
make lint

# Or install and run manually
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
golangci-lint run ./...
```

**Enabled Linters:**
- errcheck, govet, staticcheck
- gosec (security)
- gofmt, goimports (formatting)
- gocyclo (complexity)
- dupl (duplicate code)
- And 15+ more

## Release Process

### 1. Prepare Release

```bash
# Ensure on main branch
git checkout main
git pull

# Run final CI checks
make ci
```

### 2. Create Tag

```bash
# Create annotated tag
git tag -a v1.0.0 -m "Release v1.0.0"
git push origin v1.0.0
```

### 3. Verify Release

GitHub Actions will:
1. Build all platform binaries
2. Create a GitHub Release
3. Build and push Docker images

### 4. Post-Release

- Verify release notes
- Test download links
- Verify Docker images: `docker pull mordenhost/whm2bunny:latest`

## Docker Images

Images are published to Docker Hub:
- `mordenhost/whm2bunny:latest` - Latest release
- `mordenhost/whm2bunny:v1.0.0` - Versioned releases

**Platforms:** linux/amd64, linux/arm64

## Badge

Add CI status badge to README:

```markdown
[![Test](https://github.com/mordenhost/whm2bunny/actions/workflows/test.yml/badge.svg)](https://github.com/mordenhost/whm2bunny/actions/workflows/test.yml)
[![Release](https://github.com/mordenhost/whm2bunny/actions/workflows/release.yml/badge.svg)](https://github.com/mordenhost/whm2bunny/actions/workflows/release.yml)
```

## Troubleshooting

### CI Fails on Test

1. Check test output in GitHub Actions
2. Run locally: `go test ./... -v`
3. Check for race conditions: `go test -race ./...`

### Lint Fails

1. Run locally: `make lint`
2. Fix issues or add `//nolint` comments where appropriate
3. Check `.golangci.yml` for enabled rules

### Security Scan Fails

1. Review security finding in detail
2. If false positive, add `#nosec GXXX` comment
3. If real issue, fix the code

### Release Fails

1. Check tag format (must start with 'v')
2. Verify all CI checks pass on main
3. Check Docker Hub credentials in secrets
