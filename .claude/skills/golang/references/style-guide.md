# Go Style Guide Reference

Detailed style rules for writing clear, idiomatic Go code.

## Line Length and Breaking

Soft limit of ~120 characters. Break at semantic boundaries, not arbitrary columns.

Function calls with 4+ arguments MUST use one argument per line:

```go
mux.HandleFunc("/api/users", func(w http.ResponseWriter, r *http.Request) {
    handleUsers(
        w,
        r,
        serviceName,
        cfg,
        logger,
    )
})
```

When a function signature is too long, consider fewer parameters (options struct) rather than better wrapping.

## Variable Declarations

### `:=` vs `var`

The form signals intent -- `var` means "this starts at zero":

```go
var count int              // zero value, set later
name := "default"          // non-zero, := is appropriate
var buf bytes.Buffer       // zero value is ready to use
```

### Slice and Map Initialization

Initialize explicitly to avoid nil-related surprises:

```go
users := []User{}                       // empty, not nil (JSON serializes to [])
m := map[string]int{}                   // empty, not nil (nil map panics on write)
users := make([]User, 0, len(ids))      // preallocate when capacity is known
m := make(map[string]int, len(items))   // preallocate when size is known
```

Do NOT preallocate speculatively -- `make([]T, 0, 1000)` wastes memory when the common case is 10 items.

### Zero Value Structs

Use `var` for zero-value structs:

```go
// Good
var user User

// Bad -- empty literal is noisier with no benefit
user := User{}
```

### Struct References

Use `&T{}` instead of `new(T)` for consistency:

```go
// Good
sptr := &T{Name: "bar"}

// Bad
sptr := new(T)
sptr.Name = "bar"
```

## Control Flow

### Early Returns

Handle errors and edge cases first. Keep the happy path at minimal indentation:

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

### Eliminate Unnecessary Else

When the `if` body ends with `return`/`break`/`continue`, drop the `else`:

```go
// Good -- default-then-override
level := slog.LevelInfo
if debug {
    level = slog.LevelDebug
}

// Bad -- unnecessary else
if debug {
    level = slog.LevelDebug
} else {
    level = slog.LevelInfo
}
```

### Named Booleans for Complex Conditions

When an `if` has 3+ operands, extract into named booleans:

```go
isAdmin := user.Role == RoleAdmin
isOwner := resource.OwnerID == user.ID
isPublicVerified := resource.IsPublic && user.IsVerified
if isAdmin || isOwner || isPublicVerified {
    allow()
}
```

### Switch Over If-Else Chains

```go
switch status {
case StatusActive:
    activate()
case StatusInactive:
    deactivate()
default:
    return fmt.Errorf("unexpected status: %d", status)
}
```

### Scope Variables to If Blocks

```go
if err := validate(input); err != nil {
    return err
}
```

## Value vs Pointer Arguments

| Pass by | When |
|---------|------|
| Value | Small types: `string`, `int`, `bool`, `time.Time` |
| Pointer | Mutating the argument |
| Pointer | Large structs (~128+ bytes) |
| Pointer | When `nil` is a meaningful signal |
| Value | When immutability is desired |

## Import Organization

Two groups, blank line between:

```go
import (
    "context"
    "fmt"
    "net/http"

    "github.com/org/project/internal/handler"
    "go.uber.org/zap"
)
```

- Standard library first
- Everything else second
- Use `goimports` to manage automatically
- Alias imports ONLY on collision: `mrand "math/rand"`
- NEVER use dot imports in library code

## String Handling

| Need | Use |
|------|-----|
| Simple conversion | `strconv` (faster than `fmt`) |
| Complex formatting | `fmt.Sprintf` |
| Error messages with boundaries | `%q` (shows string boundaries) |
| Loop concatenation | `strings.Builder` |
| Simple concatenation | `+` operator |

## Type Conversions

Prefer explicit, narrow conversions. Use generics over `any` when a concrete type will do:

```go
func Contains[T comparable](slice []T, target T) bool {
    for _, v := range slice {
        if v == target {
            return true
        }
    }
    return false
}
```

## Field Tags in Marshaled Structs

MUST annotate all struct fields with relevant tags for serialization:

```go
type User struct {
    Name     string `json:"name"`
    Email    string `json:"email"`
    IsActive bool   `json:"is_active"`
}
```

This makes the contract explicit and guards against breaking serialized form during refactoring.

## Functional Options Pattern

Use for constructors with optional parameters:

```go
type Option func(*options)

type options struct {
    port   int
    logger *slog.Logger
}

func WithPort(port int) Option {
    return func(o *options) { o.port = port }
}

func WithLogger(logger *slog.Logger) Option {
    return func(o *options) { o.logger = logger }
}

func NewServer(addr string, opts ...Option) *Server {
    o := options{
        port:   8080,
        logger: slog.Default(),
    }
    for _, opt := range opts {
        opt(&o)
    }
    return &Server{addr: addr, port: o.port, logger: o.logger}
}
```

## Code Organization Within Files

1. Package documentation comment
2. Imports
3. Constants and package-level variables
4. Types (exported first)
5. Constructors (`New`/`NewXYZ`)
6. Methods (grouped by receiver)
7. Helper functions (unexported, toward end)

One primary type per file when it has significant methods. Blank imports (`_ "pkg"`) only in `main` and test packages.

## Sources

- [Uber Go Style Guide](https://github.com/uber-go/guide) -- Line length, imports, style section
- [samber/cc-skills-golang](https://github.com/samber/cc-skills-golang) -- golang-code-style skill
- [Effective Go](https://go.dev/doc/effective_go) -- Official guidance
