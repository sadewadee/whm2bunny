# whm2bunny Testing Documentation

## Overview

whm2bunny is tested at multiple levels to ensure reliability and correctness:
- **Unit Tests**: Individual package testing with mocks
- **Integration Tests**: End-to-end workflow testing
- **Security Tests**: HMAC verification and input validation
- **Build Verification**: Compilation and binary testing

## Test Coverage

| Package | Coverage | Description |
|---------|----------|-------------|
| `config` | 94.2% | Configuration loading and validation |
| `retry` | 91.1% | Exponential backoff and retry logic |
| `webhook` | 82.2% | HMAC verification and event routing |
| `state` | 58.9% | State persistence and recovery |
| `notifier` | 61.3% | Telegram notifications |
| `scheduler` | 33.9% | Cron jobs and summaries |
| `validator` | 100.0% | Input validation |

## Running Tests

### All Tests
```bash
go test ./... -v
```

### With Coverage
```bash
go test ./... -cover -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html
```

### Specific Package
```bash
go test ./internal/webhook/... -v
go test ./config/... -v
```

### Race Detection
```bash
go test -race ./...
```

### Benchmarks
```bash
go test -bench=. ./internal/retry/...
```

## Test Categories

### 1. Unit Tests

#### Config Package (`config/`)
Tests configuration loading from YAML files with environment variable substitution.

```bash
go test ./config/... -v
```

Key tests:
- `TestLoadDefaults` - Default configuration values
- `TestLoadFromFile` - YAML file parsing
- `TestEnvSubstitute` - Environment variable substitution
- `TestValidateMissingRequired` - Required field validation

#### Webhook Package (`internal/webhook/`)
Tests HMAC-SHA256 signature verification and event routing.

```bash
go test ./internal/webhook/... -v
```

Key tests:
- `TestHMACVerification` - Signature verification
- `TestEventRouting` - Event type routing
- `TestPayloadValidation` - Input validation
- `TestAsyncProcessing` - 202 Accepted response

#### State Package (`internal/state/`)
Tests state persistence and crash recovery.

```bash
go test ./internal/state/... -v
```

Key tests:
- `TestStatePersistence` - JSON file storage
- `TestRecovery` - Resume from crash
- `TestConcurrentAccess` - Thread safety

### 2. Integration Tests

Integration tests are located in `tests/integration/` and require:
- Running Bunny.net API (or mock server)
- Test Telegram bot token
- Test configuration file

```bash
# Run integration tests
go test ./tests/integration/... -v -tags=integration
```

### 3. Security Tests

#### HMAC Signature Verification
```bash
# Test valid signature
./scripts/test_hook.sh account_created test.example.com

# Test invalid signature (should fail)
WHM_HOOK_SECRET=wrong-secret ./scripts/test_hook.sh account_created test.example.com
```

#### Input Validation
```bash
go test ./internal/validator/... -v
```

### 4. Build Tests

```bash
# Build all platforms
make build

# Build for specific platform
GOOS=linux GOARCH=amd64 go build -o whm2bunny-linux-amd64 ./cmd/whm2bunny

# Verify binary
./whm2bunny version
./whm2bunny config generate /tmp/test-config.yaml
```

## Test Fixtures

Test fixtures are located in `tests/fixtures/`:

```
tests/fixtures/
├── configs/
│   ├── valid.yaml
│   ├── missing_api_key.yaml
│   └── invalid.yaml
├── webhooks/
│   ├── account_created.json
│   ├── subdomain_created.json
│   └── account_deleted.json
└── responses/
    ├── dns_zone_created.json
    └── pull_zone_created.json
```

## Mocking

### Bunny API Mock
```go
// Create mock server
server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    switch r.URL.Path {
    case "/dnszone":
        w.WriteHeader(http.StatusCreated)
        json.NewEncoder(w).Encode(&bunny.DNSZone{ID: 12345})
    case "/pullzone":
        w.WriteHeader(http.StatusCreated)
        json.NewEncoder(w).Encode(&bunny.PullZone{ID: 67890})
    }
}))
defer server.Close()

client := bunny.NewClient("test-key", bunny.WithBaseURL(server.URL))
```

### Telegram Mock
```go
// Test with disabled Telegram
notifier, _ := notifier.NewTelegramNotifier("", "", false, nil, logger)
// All send operations return nil without error
```

## Continuous Integration

CI/CD is configured via GitHub Actions (see `.github/workflows/`):

- **Test Pipeline**: Runs on every push/PR
- **Build Pipeline**: Builds for Linux/macOS
- **Release Pipeline**: Creates releases on tags

### CI Commands
```bash
# Local CI simulation
make ci

# Or step by step
make lint
make test
make build
```

## Writing New Tests

### Test Structure
```go
func TestMyFeature(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    string
        wantErr bool
    }{
        {
            name:  "success case",
            input: "example.com",
            want:  "valid",
        },
        {
            name:    "error case",
            input:   "",
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := MyFunction(tt.input)
            if tt.wantErr {
                assert.Error(t, err)
                return
            }
            assert.NoError(t, err)
            assert.Equal(t, tt.want, got)
        })
    }
}
```

### Best Practices

1. **Use table-driven tests** for multiple scenarios
2. **Use testify/assert** for readable assertions
3. **Create mocks** for external dependencies
4. **Test error paths** not just happy paths
5. **Use t.Parallel()** for independent tests
6. **Clean up resources** with t.Cleanup()

## Debugging Failed Tests

### Verbose Output
```bash
go test ./... -v -run TestSpecificFunction
```

### Check Test Output
```bash
go test ./... -v 2>&1 | tee test-output.log
```

### Run Specific Test
```bash
go test ./internal/webhook/... -v -run TestHMACVerification
```

### Debug with Delve
```bash
dlv test ./internal/webhook/... -- -test.run TestHMACVerification
```

## Performance Testing

### Benchmarks
```bash
go test -bench=. -benchmem ./internal/retry/...
```

### Memory Profiling
```bash
go test -memprofile=mem.out ./...
go tool pprof mem.out
```

### CPU Profiling
```bash
go test -cpuprofile=cpu.out ./...
go tool pprof cpu.out
```

## Troubleshooting

### Common Issues

1. **"undefined: fmt"** - Add `fmt` to imports
2. **Test hangs** - Check for blocking operations, use context with timeout
3. **Flaky tests** - Check for race conditions with `go test -race`
4. **Permission denied** - Ensure test directories are writable

### Reset Test Cache
```bash
go clean -testcache
```

### Check Test Dependencies
```bash
go mod tidy
go mod verify
```
