---
name: golang
description: Go code generation, project layout, naming, style, error handling, testing, concurrency, performance, and security following idiomatic Go conventions
user-invocable: true
allowed-tools:
  - Read
  - Edit
  - Write
  - Bash
  - Grep
  - Glob
  - Agent
---

# Go Engineer Skill

You are an idiomatic Go engineer. Write code that is clear, simple, and correct. Follow the Go proverbs: "Clear is better than clever", "A little copying is better than a little dependency", "Don't communicate by sharing memory; share memory by communicating."

## Workflow

1. **Analyze** -- Understand requirements and existing code (`go.mod`, project layout, conventions)
2. **Research** -- Check existing packages, interfaces, and patterns in the codebase
3. **Implement** -- Write code following all conventions below
4. **Validate** -- Run `go vet`, `go test -race ./...`, and suggest `golangci-lint run`

## Project Layout

### Module Naming

```
module github.com/<USER>/<PROJECT-NAME>
```

- MUST match repository URL
- Lowercase only, hyphens for multi-word names
- NEVER use generic names (`utils`, `common`, `shared`, `lib`)

### Directory Structure

```
cmd/
  <app-name>/
    main.go              # Minimal: parse flags, wire dependencies, call Run()
internal/                # Private packages (compiler-enforced)
  <domain>/
    <domain>.go
    <domain>_test.go
pkg/                     # Public libraries (only if external consumers exist)
api/                     # API definitions (OpenAPI specs, protobuf)
web/                     # Web assets (templates, static files)
testdata/                # Test fixtures
Makefile                 # Build automation
.golangci.yml            # Linter configuration
```

- All `main` packages MUST reside in `cmd/` with minimal logic
- Business logic belongs in `internal/` or `pkg/`
- Use `internal/` by default -- you can always export later; unexporting is a breaking change
- Co-locate `_test.go` files with the code they test
- Use `testdata/` for test fixtures

For small projects (CLI tools, scripts), a flat layout is acceptable. NEVER over-structure.

See `references/project-layout.md` for detailed examples by project type.

## Naming Conventions

### Quick Reference

| Element | Convention | Example |
|---------|-----------|---------|
| Package | lowercase, single word, singular | `json`, `http`, `user` |
| File | lowercase, underscores OK | `user_handler.go` |
| Exported name | UpperCamelCase | `ReadAll`, `HTTPClient` |
| Unexported | lowerCamelCase | `parseToken`, `userCount` |
| Interface | method + `-er` suffix | `Reader`, `Closer`, `Stringer` |
| Struct | MixedCaps noun | `Request`, `FileHeader` |
| Constant | MixedCaps (NOT `ALL_CAPS`) | `MaxRetries`, `defaultTimeout` |
| Receiver | 1-2 letter abbreviation | `func (s *Server)`, `func (b *Buffer)` |
| Error variable | `Err` prefix | `ErrNotFound`, `ErrTimeout` |
| Error type | `Error` suffix | `PathError`, `SyntaxError` |
| Constructor | `New` (single type) or `NewTypeName` | `ring.New`, `http.NewRequest` |
| Boolean field | `is`/`has`/`can` prefix | `isReady`, `IsConnected()` |
| Acronym | all caps or all lower | `URL`, `HTTPServer`, `xmlParser` |
| Enum (iota) | type prefix, zero = unknown | `StatusUnknown` at 0 |
| Error string | lowercase, no punctuation | `"image: unknown format"` |
| Option func | `With` + field name | `WithPort()`, `WithLogger()` |

### Key Rules

- All identifiers MUST use `MixedCaps` -- NEVER underscores (except test subcases `TestFoo_InvalidInput`)
- Constants MUST NOT use `ALL_CAPS` -- Go reserves casing for visibility, not emphasis
- Avoid stuttering: `http.Client` not `http.HTTPClient`, `user.New()` not `user.NewUser()`
- Getters omit `Get`: `user.Name()` not `user.GetName()` -- but keep `Is`/`Has`/`Can` for booleans
- Receivers: consistent 1-2 letter name across all methods of a type; NEVER `this` or `self`
- Enum zero values: always place `Unknown`/`Invalid` sentinel at iota position 0

See `references/naming-conventions.md` for detailed rules and common mistakes.

## Code Style

### Variable Declarations

Use `:=` for non-zero values, `var` for zero-value initialization:

```go
var count int              // zero value, set later
name := "default"          // non-zero, := is appropriate
var buf bytes.Buffer       // zero value is ready to use
```

### Composite Literals

MUST use field names -- positional fields break on type changes:

```go
srv := &http.Server{
    Addr:         ":8080",
    ReadTimeout:  5 * time.Second,
    WriteTimeout: 10 * time.Second,
}
```

### Control Flow

- Handle errors first, return early -- keep the happy path at minimal indentation
- When `if` body ends with `return`/`break`/`continue`, drop the `else`
- Prefer `switch` over if-else chains when comparing the same variable
- Extract complex conditions (3+ operands) into named booleans

```go
func process(data []byte) (*Result, error) {
    if len(data) == 0 {
        return nil, errors.New("empty data")
    }

    parsed, err := parse(data)
    if err != nil {
        return nil, fmt.Errorf("parsing: %w", err)
    }

    return transform(parsed), nil
}
```

### Function Design

- Functions SHOULD have 4 or fewer parameters -- beyond that, use an options struct
- Parameter order: `context.Context` first, then inputs, then output destinations
- One function, one job -- keep functions short and focused
- Prefer `range` for iteration; use `range n` (Go 1.22+) for counting

### Line Length

No rigid limit, but lines beyond ~120 characters SHOULD be broken at semantic boundaries. Function calls with 4+ arguments: one argument per line.

### Imports

Two groups separated by blank line:
1. Standard library
2. Everything else

Use `goimports` to manage import grouping automatically.

### Code Organization Within Files

Order: package doc, imports, constants, types, constructors, methods, helpers. Group related declarations. One primary type per file when it has significant methods.

See `references/style-guide.md` for detailed style rules.

## Error Handling

### Core Rules

1. Returned errors MUST always be checked -- NEVER discard with `_`
2. Errors MUST be wrapped with context: `fmt.Errorf("doing X: %w", err)`
3. Error strings MUST be lowercase, without trailing punctuation
4. Errors MUST be either logged OR returned, NEVER both (single handling rule)
5. Use `errors.Is` and `errors.As` -- NEVER direct comparison or type assertion
6. Use `%w` internally, `%v` at system boundaries to control error chain exposure

### Error Creation Decision Table

| Need matching? | Message | Approach |
|----------------|---------|----------|
| No | Static | `errors.New("msg")` |
| No | Dynamic | `fmt.Errorf("msg: %v", val)` |
| Yes | Static | Top-level `var ErrX = errors.New("msg")` |
| Yes | Dynamic | Custom error type |

### Don't Panic

Production code MUST NOT panic for expected conditions. Return errors. Reserve `panic` for truly unrecoverable states. In `main()`, use `log.Fatal` only at the top level:

```go
func main() {
    if err := run(); err != nil {
        log.Fatal(err)
    }
}
```

See `references/error-patterns.md` for wrapping patterns, sentinel errors, and custom types.

## Testing

### Core Rules

1. Table-driven tests MUST use named subtests via `t.Run`
2. Integration tests MUST use build tags (`//go:build integration`)
3. Tests MUST NOT depend on execution order
4. Independent tests SHOULD use `t.Parallel()`
5. Test observable behavior and public API contracts -- NEVER implementation details
6. Use `go.uber.org/goleak` to detect goroutine leaks

### Table-Driven Tests

```go
func TestCalculatePrice(t *testing.T) {
    tests := []struct {
        name     string
        quantity int
        price    float64
        expected float64
    }{
        {name: "single item", quantity: 1, price: 10.0, expected: 10.0},
        {name: "bulk discount", quantity: 100, price: 10.0, expected: 900.0},
        {name: "zero quantity", quantity: 0, price: 10.0, expected: 0.0},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := CalculatePrice(tt.quantity, tt.price)
            if got != tt.expected {
                t.Errorf("got %.2f, want %.2f", got, tt.expected)
            }
        })
    }
}
```

### Quick Reference

```bash
go test ./...                          # all tests
go test -run TestName ./...            # specific test
go test -race ./...                    # race detection
go test -cover ./...                   # coverage summary
go test -bench=. -benchmem ./...       # benchmarks
go test -fuzz=FuzzName ./...           # fuzzing
go test -tags=integration ./...        # integration tests
go test -coverprofile=c.out ./...      # coverage file
go tool cover -html=c.out             # coverage HTML
```

See `references/testing-patterns.md` for HTTP handler tests, mocking, benchmarks, fuzzing, and fixtures.

## Concurrency

### Core Principles

1. Every goroutine MUST have a clear exit mechanism (context, done channel, WaitGroup)
2. Share memory by communicating -- prefer channels over shared state
3. Only the sender closes a channel
4. Specify channel direction (`chan<-`, `<-chan`)
5. Default to unbuffered channels
6. Always include `ctx.Done()` in `select`
7. NEVER use `time.After` in loops -- use `time.NewTimer` + `Reset`

### Channel vs Mutex vs Atomic

| Scenario | Use | Why |
|----------|-----|-----|
| Passing data between goroutines | Channel | Communicates ownership transfer |
| Coordinating goroutine lifecycle | Channel + context | Clean shutdown with select |
| Protecting shared struct fields | `sync.Mutex` / `sync.RWMutex` | Simple critical sections |
| Simple counters, flags | `sync/atomic` | Lock-free, lower overhead |
| Many readers, few writers on a map | `sync.Map` | Optimized for read-heavy workloads |
| Caching expensive computations | `sync.Once` / `singleflight` | Execute once or deduplicate |

### Concurrency Checklist

Before spawning a goroutine, answer:

- [ ] How will it exit?
- [ ] Can I signal it to stop?
- [ ] Can I wait for it?
- [ ] Who owns the channels?
- [ ] Should this be synchronous instead?

See `references/concurrency-patterns.md` for pipelines, worker pools, errgroup, and sync primitives.

## Performance

Apply only to hot paths -- do NOT optimize speculatively.

### Key Rules

- Preallocate slices and maps when size is known: `make([]T, 0, n)`
- Prefer `strconv` over `fmt` for simple conversions (2x faster)
- Avoid repeated string-to-byte conversions -- convert once and reuse
- Use `strings.Builder` for string concatenation in loops
- Specify container capacity: `make(map[K]V, hint)`
- Use `b.ReportAllocs()` in benchmarks to track allocations
- Profile before optimizing: `go tool pprof`

### Data Structure Selection

| Need | Use | Why |
|------|-----|-----|
| Ordered collection, random access | Slice | Cache-friendly, growable |
| Key-value lookup | Map | O(1) average access |
| Fixed-size, compile-time known | Array | Value type, usable as map key |
| Priority queue | `container/heap` | Efficient insert/extract-min |
| String building | `strings.Builder` | No copy on `String()` |
| Bidirectional I/O | `bytes.Buffer` | Implements `io.Reader` and `io.Writer` |

See `references/style-guide.md` for value vs pointer argument guidelines.

## Security

### Critical Rules

- NEVER use `math/rand` for tokens or secrets -- use `crypto/rand`
- NEVER concatenate SQL strings -- use parameterized queries (`database/sql` with `?`)
- NEVER use `exec.Command("bash", "-c", userInput)` -- pass args separately
- NEVER hardcode secrets -- use environment variables or secret managers
- Use `html/template` for web output (auto-escaping), NEVER `text/template`
- Compare secrets with `crypto/subtle.ConstantTimeCompare`, not `==`
- Always run `go test -race ./...` in CI
- Run `govulncheck ./...` to check for known vulnerabilities

### Quick Reference

| Severity | Vulnerability | Defense |
|----------|--------------|---------|
| Critical | SQL injection | Parameterized queries with `database/sql` |
| Critical | Command injection | `exec.Command` with separate args |
| Critical | Hardcoded secrets | Environment variables or secret managers |
| High | XSS | `html/template` auto-escaping |
| High | Path traversal | `os.Root` (Go 1.24+), `filepath.Clean` |
| High | Weak crypto | `crypto/aes` GCM, `crypto/rand` |
| Medium | Timing attacks | `crypto/subtle.ConstantTimeCompare` |
| High | Race conditions | `sync.Mutex`, channels, `-race` flag |

See `references/security-checklist.md` for the full security review checklist.

## Validation Pipeline

```bash
gofmt -s -w .                    # format
goimports -w .                   # organize imports
go vet ./...                     # static analysis
golangci-lint run                # comprehensive linting
go test -race -cover ./...       # test with race detection
govulncheck ./...                # vulnerability scan
```

### Recommended Linters (golangci-lint)

Minimum set: `errcheck`, `govet`, `staticcheck`, `revive`, `goimports`. Add `gosec` for security analysis.

## DO NOTs

- Do NOT use `panic` for expected error conditions
- Do NOT discard errors with `_` (except explicitly justified cases)
- Do NOT use `init()` unless deterministic and side-effect-free
- Do NOT fire-and-forget goroutines -- every goroutine needs a shutdown mechanism
- Do NOT use `ALL_CAPS` for constants
- Do NOT use `this`/`self` for receivers
- Do NOT shadow built-in names (`error`, `string`, `len`, `cap`)
- Do NOT use mutable globals -- prefer dependency injection
- Do NOT embed types in public structs without careful consideration
- Do NOT use `reflect` unless absolutely necessary

## IaC Tooling and Kubernetes Operators

When developing Terraform providers, Kubernetes operators, or other IaC tooling in Go, apply these additional patterns.

### Terraform Provider Development

- Use the `terraform-plugin-framework` (not the deprecated SDKv2) for new providers
- Follow the `terraform-plugin-framework` resource lifecycle: `Create`, `Read`, `Update`, `Delete`
- Implement `ImportState` for all resources
- Use `terraform-plugin-testing` for acceptance tests with `resource.Test` and `resource.TestStep`
- Provider schemas MUST match the API 1:1 -- do NOT add computed convenience fields
- Use `context.Context` propagation in all CRUD methods
- Acceptance tests MUST be integration tests with real infrastructure (use build tags)

```go
func (r *ExampleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
    var data ExampleResourceModel
    resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
    if resp.Diagnostics.HasError() {
        return
    }
    // API call, map response to state
    resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
```

### Kubernetes Operator Development

- Use `controller-runtime` (kubebuilder/operator-sdk) for operator scaffolding
- Reconcile loops MUST be idempotent -- same input produces same output regardless of current state
- Use `controllerutil.SetControllerReference` for owner references (automatic garbage collection)
- Implement `Finalizers` for cleanup of external resources
- Use `Status` subresource for reporting state (NOT spec fields)
- Use `controller-runtime`'s `client.Client` for API interactions (not `client-go` directly)
- CRDs MUST have validation via OpenAPI schema (kubebuilder markers)
- Use `envtest` for integration tests (spins up a real API server, no cluster needed)

```go
func (r *MyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var obj MyResource
    if err := r.Get(ctx, req.NamespacedName, &obj); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Idempotent reconciliation logic

    return ctrl.Result{}, nil
}
```

### Common Patterns for Both

- Structured logging with `slog` or `logr` (controller-runtime's logger interface)
- Exponential backoff for API calls with `wait.ExponentialBackoff` or `ctrl.Result{RequeueAfter: ...}`
- Context propagation throughout the call chain
- Integration tests with real backends (not mocks for provider/operator behavior)
- `make generate` for code generation (deepcopy, CRD manifests, provider schemas)

See `references/iac-tooling.md` for detailed patterns.

## Philosophy -- Go Proverbs

The [Go Proverbs](https://go-proverbs.github.io/) by Rob Pike capture the essence of Go's design philosophy. These are not suggestions -- they are the cultural foundation of the language:

- **"Clear is better than clever."** -- Readability wins over elegance. If someone has to think hard to understand your code, simplify it.
- **"Don't communicate by sharing memory, share memory by communicating."** -- Use channels to transfer ownership, not mutexes to guard shared state.
- **"Concurrency is not parallelism."** -- Concurrency is about structure; parallelism is about execution. Design for concurrency, the runtime handles parallelism.
- **"Channels orchestrate; mutexes serialize."** -- Channels coordinate goroutine lifecycles; mutexes protect data. Choose based on the problem.
- **"The bigger the interface, the weaker the abstraction."** -- Small interfaces (`io.Reader`, `io.Writer`) are powerful. Large interfaces are hard to implement and hard to mock.
- **"Make the zero value useful."** -- `var buf bytes.Buffer` is ready to use. Design your types the same way.
- **"interface{} says nothing."** -- Use generics or concrete types. `any` erases type information and pushes errors to runtime.
- **"Errors are values."** -- Errors are not exceptions. They are regular values that can be inspected, compared, wrapped, and returned.
- **"Don't just check errors, handle them gracefully."** -- Wrap with context, return to the caller, or handle and recover. NEVER silently discard.
- **"A little copying is better than a little dependency."** -- A 5-line helper function copied into your project is better than importing a 500-line package.
- **"Reflection is never clear."** -- Avoid `reflect` unless absolutely necessary. It defeats type safety and confuses readers.
- **"Gofmt's style is no one's favorite, yet gofmt is everyone's favorite."** -- Consistency beats personal preference. Run `gofmt` and move on.
- **"Don't panic."** -- Return errors. Panics are for truly unrecoverable states, not for input validation.
- **"Design the architecture, name the components, document the details."** -- Architecture is about structure, naming is about clarity, documentation is about communication.
- **"Documentation is for users."** -- Write documentation that helps the consumer of your API, not the author.
- **"Cgo is not Go."** -- Cgo introduces build complexity, platform dependencies, and GC interaction issues. Avoid unless necessary.
- **"With the unsafe package there are no guarantees."** -- The `unsafe` package voids Go's memory safety guarantees. Use only with extreme care.
- **"Syscall must always be guarded with build tags."** -- Platform-specific code must be conditionally compiled.

## Inspirations and References

This skill synthesizes best practices from:

- **[Uber Go Style Guide](https://github.com/uber-go/guide)** -- Production-tested conventions from one of the largest Go codebases
- **[samber/cc-skills-golang](https://github.com/samber/cc-skills-golang)** -- Comprehensive Claude Code skills for Go development (35 skills, 98% effectiveness)
- **[Effective Go](https://go.dev/doc/effective_go)** -- Official Go team guidance
- **[Go Code Review Comments](https://go.dev/wiki/CodeReviewComments)** -- Community wiki of review patterns
- **[Go Proverbs](https://go-proverbs.github.io/)** -- Rob Pike's guiding principles
- **[cloudflare/terraform-provider-cloudflare](https://github.com/cloudflare/terraform-provider-cloudflare)** -- Reference implementation for large-scale Terraform providers (service-per-resource, schema patterns, test templates)

## References

See `references/` directory for:
- `style-guide.md` -- Detailed style rules, value vs pointer, imports, line breaking
- `naming-conventions.md` -- Comprehensive naming rules with examples and common mistakes
- `error-patterns.md` -- Error wrapping, sentinel errors, custom types, structured logging
- `testing-patterns.md` -- HTTP handler tests, mocking, benchmarks, fuzzing, fixtures
- `concurrency-patterns.md` -- Pipelines, worker pools, errgroup, sync primitives
- `security-checklist.md` -- Full security review checklist by domain
- `project-layout.md` -- Project structure examples by project type
- `terraform-provider.md` -- Terraform provider development (CRUD lifecycle, schemas, testing, service-per-resource)
- `kubernetes-operator.md` -- Kubernetes operator development (reconcile loops, CRDs, envtest, finalizers)
- `iac-tooling.md` -- Shared IaC patterns (API client, retry, logging, context propagation)
