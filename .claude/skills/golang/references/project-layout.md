# Go Project Layout Reference

Project structure examples by type and complexity.

## Decision: Ask First

NEVER over-structure. Ask the developer about preferred architecture (clean, hexagonal, DDD, flat) and dependency injection approach (manual, wire, dig/fx, none) before creating the layout.

## CLI Tool

```
<PROJECT>/
├── cmd/
│   └── <APP_NAME>/
│       └── main.go          # Parse flags, wire deps, call run()
├── internal/
│   ├── cli/
│   │   └── root.go          # Cobra root command
│   ├── config/
│   │   └── config.go        # Configuration loading
│   └── <DOMAIN>/
│       ├── <DOMAIN>.go
│       └── <DOMAIN>_test.go
├── go.mod
├── go.sum
├── Makefile
├── .golangci.yml
└── .gitignore
```

### main.go Pattern

```go
package main

import (
    "fmt"
    "os"

    "github.com/<USER>/<PROJECT>/internal/cli"
)

func main() {
    if err := cli.Execute(); err != nil {
        fmt.Fprintf(os.Stderr, "error: %v\n", err)
        os.Exit(1)
    }
}
```

## Library

```
<PROJECT>/
├── <PACKAGE>.go             # Primary public API
├── <PACKAGE>_test.go        # Tests
├── options.go               # Functional options (if needed)
├── internal/                # Private implementation
│   └── <IMPL>/
├── testdata/                # Test fixtures
├── example_test.go          # Executable examples
├── go.mod
├── go.sum
└── .golangci.yml
```

Rules for libraries:
- Keep the public API surface minimal
- Co-locate examples with tests
- Use `internal/` for implementation details
- Provide `example_test.go` as executable documentation

## HTTP Service

```
<PROJECT>/
├── cmd/
│   └── <SERVICE_NAME>/
│       └── main.go
├── internal/
│   ├── handler/             # HTTP handlers
│   │   ├── handler.go
│   │   ├── handler_test.go
│   │   └── middleware.go
│   ├── service/             # Business logic
│   │   ├── service.go
│   │   └── service_test.go
│   ├── repository/          # Data access
│   │   ├── repository.go
│   │   └── repository_test.go
│   ├── model/               # Domain types
│   │   └── model.go
│   └── config/
│       └── config.go
├── api/                     # OpenAPI specs, protobuf
│   └── openapi.yaml
├── migrations/              # Database migrations
├── testdata/                # Test fixtures
├── go.mod
├── go.sum
├── Makefile
├── Dockerfile
├── .golangci.yml
└── .gitignore
```

### main.go Pattern for Services

```go
package main

import (
    "context"
    "log/slog"
    "os"
    "os/signal"
    "syscall"

    "github.com/<USER>/<PROJECT>/internal/config"
)

func main() {
    if err := run(); err != nil {
        slog.Error("fatal", "error", err)
        os.Exit(1)
    }
}

func run() error {
    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()

    cfg, err := config.Load()
    if err != nil {
        return fmt.Errorf("loading config: %w", err)
    }

    // Wire dependencies, start server, wait for shutdown signal
    return serve(ctx, cfg)
}
```

## Monorepo

```
<PROJECT>/
├── go.work                  # Workspace file
├── go.work.sum
├── services/
│   ├── <SERVICE_A>/
│   │   ├── cmd/
│   │   ├── internal/
│   │   ├── go.mod
│   │   └── go.sum
│   └── <SERVICE_B>/
│       ├── cmd/
│       ├── internal/
│       ├── go.mod
│       └── go.sum
├── pkg/                     # Shared libraries
│   └── <SHARED_LIB>/
│       ├── go.mod
│       └── go.sum
└── tools/                   # Development tools
    └── go.mod
```

### go.work Setup

```go
go 1.22

use (
    ./services/<SERVICE_A>
    ./services/<SERVICE_B>
    ./pkg/<SHARED_LIB>
)
```

## Small Project (Flat Layout)

For simple scripts, small CLIs, or single-purpose tools:

```
<PROJECT>/
├── main.go
├── main_test.go
├── handler.go               # Additional files as needed
├── handler_test.go
├── go.mod
└── go.sum
```

This is perfectly fine. Not every project needs `cmd/` and `internal/`.

## Essential Configuration Files

### Makefile

```makefile
.PHONY: build test lint run

build:
	go build -o bin/<APP_NAME> ./cmd/<APP_NAME>

test:
	go test -race -cover ./...

lint:
	golangci-lint run

run:
	go run ./cmd/<APP_NAME>

fmt:
	gofmt -s -w .
	goimports -w .

vet:
	go vet ./...

vuln:
	govulncheck ./...
```

### .gitignore

```
# Binaries
bin/
*.exe
*.exe~
*.dll
*.so
*.dylib

# Test
*.test
coverage.out
coverage.html

# Dependency
vendor/

# IDE
.idea/
.vscode/
*.swp
*.swo

# OS
.DS_Store
Thumbs.db

# Environment
.env
.env.local
```

### .golangci.yml (Minimal)

```yaml
run:
  timeout: 5m

linters:
  enable:
    - errcheck
    - govet
    - staticcheck
    - revive
    - goimports
    - gosec
    - bodyclose
    - sqlclosecheck

linters-settings:
  revive:
    rules:
      - name: exported
        arguments:
          - disableStutteringCheck
```

## Common Mistakes

| Mistake | Fix |
|---------|-----|
| Over-structuring small projects | Flat layout is fine for simple tools |
| `pkg/` with no external consumers | Use `internal/` -- `pkg/` implies public API |
| Business logic in `cmd/` | `main.go` should only wire and start |
| Multiple `main` packages outside `cmd/` | All binaries under `cmd/<name>/` |
| `utils` or `helpers` packages | Name packages by what they do |
| Missing `internal/` | Default to private; export deliberately |

## 12-Factor App Principles

For applications (services, APIs, workers):

1. **Config** via environment variables
2. **Logs** to stdout (structured with slog)
3. **Stateless** processes
4. **Graceful shutdown** on SIGTERM/SIGINT
5. **Backing services** as attached resources
6. **Admin tasks** as one-off commands (`cmd/migrate/`)
7. **Dev/prod parity** -- minimize divergence

## Sources

- [samber/cc-skills-golang](https://github.com/samber/cc-skills-golang) -- golang-project-layout skill
- [Uber Go Style Guide](https://github.com/uber-go/guide) -- Exit in main, avoid init
- [Standard Go Project Layout](https://github.com/golang-standards/project-layout) -- Community conventions
