# Go CI — Go Continuous Integration Pipeline

## Purpose

Manages the Go CI pipeline for VaultCenter and LocalVault services.

## Commands

```bash
# Full test suite
go test ./...

# Test with race detection
go test -race ./...

# Static analysis
go vet ./...

# Build all services
go build ./services/vaultcenter/...
go build ./services/localvault/...

# Test coverage
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

## CI Checks

1. **Build** — `go build ./...` must succeed with zero errors
2. **Test** — `go test ./...` all tests pass
3. **Race** — `go test -race ./...` no data races
4. **Vet** — `go vet ./...` no issues
5. **Coverage** — Track coverage, flag significant drops

## Failure Handling

- On test failure: report exact test name, file, and line number
- On build failure: report the compilation error with full context
- On race detection: report the goroutine stack traces involved
