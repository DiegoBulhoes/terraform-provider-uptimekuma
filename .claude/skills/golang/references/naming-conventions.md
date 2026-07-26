# Go Naming Conventions Reference

Comprehensive naming rules for idiomatic Go code.

## MixedCaps -- The Universal Rule

All Go identifiers MUST use `MixedCaps` (or `mixedCaps`). NEVER underscores.

Exceptions: test function subcases (`TestFoo_InvalidInput`), generated code, OS/cgo interop.

```go
// Good
MaxPacketSize
userCount
parseHTTPResponse

// Bad
MAX_PACKET_SIZE   // C/Python style
max_packet_size   // snake_case
kMaxBufferSize    // Hungarian notation
```

## Package Names

- All lowercase, single word, singular
- MUST NOT require renaming at most call sites
- NEVER use `common`, `util`, `shared`, `lib`, `helpers`
- Match directory name

```go
// Good
package json
package http
package user

// Bad
package httpUtils     // MixedCaps
package user_service  // underscores
package users         // plural
```

## Avoid Stuttering

Go call sites include the package name, so repeating it wastes the reader's time:

```go
// Good -- clean at the call site
http.Client         // not http.HTTPClient
json.Decoder        // not json.JSONDecoder
user.New()          // not user.NewUser()
config.Parse()      // not config.ParseConfig()
```

## Constructors

- Single primary type in package: `New()` (avoids stuttering)
- Multiple constructible types: `NewTypeName()` (e.g., `http.NewRequest`, `http.NewServeMux`)

```go
// Package apiclient with single primary type
func New(baseURL string) *Client { ... }   // callers write apiclient.New()

// Package http with multiple types
func NewRequest(...) *Request { ... }
func NewServeMux() *ServeMux { ... }
```

## Interfaces

- Single-method interfaces: method name + `-er` suffix (`Reader`, `Writer`, `Closer`, `Stringer`)
- Multi-method interfaces: descriptive noun (`ReadWriteCloser`, `Handler`)
- Define interfaces where consumed (not where implemented)

## Structs

- MixedCaps nouns: `Request`, `FileHeader`, `Connection`
- Do NOT include type in name: `users` not `userSlice`

## Constants and Enums

```go
// Constants: MixedCaps, never ALL_CAPS
const MaxRetries = 3
const defaultTimeout = 30 * time.Second

// Enums: type prefix, zero = unknown/invalid
type Status int

const (
    StatusUnknown  Status = iota  // zero value = sentinel
    StatusActive
    StatusInactive
)
```

## Boolean Fields and Methods

Unexported boolean fields MUST use `is`/`has`/`can` prefix:

```go
type Connection struct {
    isConnected bool
    hasTimeout  bool
    canRetry    bool
}

// Exported getter keeps the prefix
func (c *Connection) IsConnected() bool { return c.isConnected }
func (c *Connection) HasTimeout() bool  { return c.hasTimeout }
```

## Receivers

1-2 letter abbreviation of the type, consistent across all methods:

```go
func (s *Server) Start() error { ... }
func (s *Server) Stop() error { ... }
func (s *Server) Handle(pattern string, h http.Handler) { ... }
```

NEVER use `this` or `self`.

## Error Naming

```go
// Sentinel errors: Err prefix
var ErrNotFound = errors.New("user: not found")
var ErrTimeout  = errors.New("user: request timeout")

// Custom error types: Error suffix
type NotFoundError struct {
    ID string
}
func (e *NotFoundError) Error() string {
    return fmt.Sprintf("user: not found: %s", e.ID)
}
```

Error strings MUST be lowercase, including acronyms, no trailing punctuation:

```go
// Good
errors.New("invalid message id")
fmt.Errorf("parsing token: %w", err)

// Bad
errors.New("Invalid message ID.")
fmt.Errorf("Failed to parse token: %w", err)
```

## Getters and Setters

Go omits `Get` -- the field name reads naturally:

```go
// Good
func (u *User) Name() string     { return u.name }
func (u *User) SetName(n string) { u.name = n }

// Bad
func (u *User) GetName() string  { return u.name }
```

Keep `Is`/`Has`/`Can` prefixes for boolean predicates:

```go
func (u *User) IsActive() bool { return u.isActive }
```

## Acronyms

All caps or all lower -- never mixed:

```go
// Good
URL, HTTPServer, xmlParser, userID

// Bad
Url, HttpServer, XmlParser, userId
```

## Test Naming

```go
func TestAdd(t *testing.T) { ... }                // function test
func TestMyStruct_MyMethod(t *testing.T) { ... }  // method test
func BenchmarkAdd(b *testing.B) { ... }            // benchmark
func ExampleAdd() { ... }                          // example
```

Subtest names: lowercase descriptive phrases:

```go
t.Run("valid id", func(t *testing.T) { ... })
t.Run("empty input", func(t *testing.T) { ... })
```

## Function Variants

| Pattern | Meaning | Example |
|---------|---------|---------|
| `WithContext` suffix | Context-aware variant | `FetchWithContext()` |
| `Must` prefix | Panics on error | `MustParse()` |
| `In` suffix | In-place mutation | `SortIn()` |
| `f` suffix | Format string semantics | `Errorf()`, `Wrapf()` |
| `With` prefix | Functional option | `WithPort()`, `WithLogger()` |

## Import Aliasing

Only alias on collision:

```go
import (
    mrand "math/rand"
    crand "crypto/rand"
)
```

NEVER alias without a collision -- aliases add cognitive load.

## Common Mistakes

| Mistake | Fix |
|---------|-----|
| `ALL_CAPS` constants | Use `MixedCaps` (`MaxRetries`) |
| `GetName()` getter | `Name()` -- Go omits `Get` |
| `Url`, `Http` acronyms | `URL`, `HTTP` -- all caps or all lower |
| `this`/`self` receiver | 1-2 letter abbreviation (`s` for `Server`) |
| `util`, `helper` packages | Specific names that describe the abstraction |
| `connected bool` field | `isConnected` -- reads as true/false question |
| `StatusReady` at iota 0 | `StatusUnknown` at 0 catches uninitialized values |
| Long names for short scopes | `i` is fine for a 3-line loop |
| Inconsistent receiver names | One name consistently across all methods |
| `snake_case` identifiers | `mixedCaps` -- Go convention and tooling expectation |

## Sources

- [samber/cc-skills-golang](https://github.com/samber/cc-skills-golang) -- golang-naming skill
- [Uber Go Style Guide](https://github.com/uber-go/guide) -- Package names, function names, import aliasing
- [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments) -- Community conventions
