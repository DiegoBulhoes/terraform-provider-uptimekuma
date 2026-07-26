# Go Error Handling Patterns

Detailed error handling patterns for production Go code.

## The Single Handling Rule

Errors MUST be either logged OR returned, NEVER both. Doing both causes duplicate logs in aggregation tools.

```go
// Good -- wrap and return
func fetchUser(ctx context.Context, id string) (*User, error) {
    user, err := db.GetUser(ctx, id)
    if err != nil {
        return nil, fmt.Errorf("fetching user %s: %w", id, err)
    }
    return user, nil
}

// Good -- log and handle (at top level)
func handleRequest(w http.ResponseWriter, r *http.Request) {
    user, err := fetchUser(r.Context(), r.PathValue("id"))
    if err != nil {
        slog.Error("request failed", "error", err, "path", r.URL.Path)
        http.Error(w, "internal error", http.StatusInternalServerError)
        return
    }
    // ...
}

// Bad -- log AND return (duplicate logs)
func fetchUser(ctx context.Context, id string) (*User, error) {
    user, err := db.GetUser(ctx, id)
    if err != nil {
        log.Printf("failed to get user: %v", err)  // logged here
        return nil, err                               // AND returned to caller who also logs
    }
    return user, nil
}
```

## Error Wrapping

### Use `%w` for Internal Errors

`%w` wraps the error, making it inspectable with `errors.Is` and `errors.As`:

```go
func loadConfig(path string) (*Config, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("reading config %s: %w", path, err)
    }

    var cfg Config
    if err := json.Unmarshal(data, &cfg); err != nil {
        return nil, fmt.Errorf("parsing config: %w", err)
    }
    return &cfg, nil
}
```

### Use `%v` at System Boundaries

At API boundaries, use `%v` to prevent leaking implementation details:

```go
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    result, err := h.service.Process(r.Context(), r.Body)
    if err != nil {
        // %v -- don't expose internal error chain to API consumers
        slog.Error("processing failed", "error", fmt.Errorf("process: %v", err))
        http.Error(w, "internal error", http.StatusInternalServerError)
        return
    }
    // ...
}
```

### Wrapping Context Style

Keep context succinct. Describe the action, not the failure:

```go
// Good -- short action description
fmt.Errorf("reading config: %w", err)
fmt.Errorf("connecting to db: %w", err)
fmt.Errorf("parsing token: %w", err)

// Bad -- redundant "failed to" prefix
fmt.Errorf("failed to read config: %w", err)
fmt.Errorf("error connecting to database: %w", err)
```

## Sentinel Errors

Use for expected, matchable conditions:

```go
package user

var (
    ErrNotFound     = errors.New("user: not found")
    ErrUnauthorized = errors.New("user: unauthorized")
    ErrConflict     = errors.New("user: already exists")
)
```

Callers match with `errors.Is`:

```go
user, err := userService.Get(ctx, id)
if errors.Is(err, user.ErrNotFound) {
    http.Error(w, "user not found", http.StatusNotFound)
    return
}
if err != nil {
    http.Error(w, "internal error", http.StatusInternalServerError)
    return
}
```

## Custom Error Types

Use when errors carry structured data:

```go
type ValidationError struct {
    Field   string
    Message string
}

func (e *ValidationError) Error() string {
    return fmt.Sprintf("validation: %s: %s", e.Field, e.Message)
}

// Usage
func validateAge(age int) error {
    if age < 0 || age > 150 {
        return &ValidationError{Field: "age", Message: "must be between 0 and 150"}
    }
    return nil
}

// Caller inspects with errors.As
var ve *ValidationError
if errors.As(err, &ve) {
    slog.Warn("validation failed", "field", ve.Field, "message", ve.Message)
}
```

## errors.Join (Go 1.20+)

Combine independent errors:

```go
func validateUser(u User) error {
    var errs []error
    if u.Name == "" {
        errs = append(errs, errors.New("name is required"))
    }
    if u.Email == "" {
        errs = append(errs, errors.New("email is required"))
    }
    return errors.Join(errs...)
}
```

## Structured Error Logging with slog

Use `slog` (Go 1.21+) for structured error logging:

```go
slog.Error("request failed",
    "error", err,
    "user_id", userID,
    "method", r.Method,
    "path", r.URL.Path,
    "duration", time.Since(start),
)
```

### Log Levels for Errors

| Level | When |
|-------|------|
| `slog.Error` | Operation failed, requires attention |
| `slog.Warn` | Degraded but functional, unexpected condition |
| `slog.Info` | Normal operations, request completion |
| `slog.Debug` | Diagnostic detail for debugging |

### Low-Cardinality Error Messages

NEVER interpolate variable data into error strings. Attach them as structured attributes:

```go
// Good -- low cardinality, groupable by APM tools
slog.Error("user fetch failed", "error", err, "user_id", id)

// Bad -- high cardinality, ungroupable
slog.Error(fmt.Sprintf("failed to fetch user %s: %v", id, err))
```

## Panic and Recover

### When to Panic

- Program initialization failures that should abort
- Programmer errors (violated invariants) in development
- NEVER for expected error conditions in production code

### Recovery at Goroutine Boundaries

```go
func safeGo(fn func()) {
    go func() {
        defer func() {
            if r := recover(); r != nil {
                slog.Error("goroutine panic recovered",
                    "panic", r,
                    "stack", string(debug.Stack()),
                )
            }
        }()
        fn()
    }()
}
```

### Exit Only in main()

```go
func main() {
    if err := run(); err != nil {
        fmt.Fprintf(os.Stderr, "error: %v\n", err)
        os.Exit(1)
    }
}

func run() error {
    cfg, err := loadConfig()
    if err != nil {
        return fmt.Errorf("loading config: %w", err)
    }
    return serve(cfg)
}
```

## Sources

- [Uber Go Style Guide](https://github.com/uber-go/guide) -- Error types, wrapping, naming, don't panic
- [samber/cc-skills-golang](https://github.com/samber/cc-skills-golang) -- golang-error-handling skill
- [Go Blog: Working with Errors](https://go.dev/blog/go1.13-errors) -- errors.Is, errors.As, %w
