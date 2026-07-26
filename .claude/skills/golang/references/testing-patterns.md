# Go Testing Patterns

Detailed testing patterns and examples for production Go code.

## Table-Driven Tests

The idiomatic Go way to test multiple scenarios:

```go
func TestParseToken(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    Token
        wantErr bool
    }{
        {
            name:  "valid token",
            input: "Bearer abc123",
            want:  Token{Type: "Bearer", Value: "abc123"},
        },
        {
            name:    "missing prefix",
            input:   "abc123",
            wantErr: true,
        },
        {
            name:    "empty string",
            input:   "",
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := ParseToken(tt.input)
            if tt.wantErr {
                if err == nil {
                    t.Fatal("expected error, got nil")
                }
                return
            }
            if err != nil {
                t.Fatalf("unexpected error: %v", err)
            }
            if got != tt.want {
                t.Errorf("got %v, want %v", got, tt.want)
            }
        })
    }
}
```

### When NOT to Use Table Tests

- Complex branching logic inside subtests
- Each case needs significantly different setup
- Conditional assertions (if this then assert that)

In these cases, write separate test functions.

## Parallel Tests

Use `t.Parallel()` for independent tests:

```go
func TestOperations(t *testing.T) {
    tests := []struct {
        name string
        data []byte
    }{
        {name: "small data", data: make([]byte, 1024)},
        {name: "large data", data: make([]byte, 1024*1024)},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            t.Parallel()
            result := Process(tt.data)
            if result == nil {
                t.Error("expected non-nil result")
            }
        })
    }
}
```

## HTTP Handler Tests

Use `httptest` for handler tests:

```go
func TestGetUser(t *testing.T) {
    tests := []struct {
        name       string
        userID     string
        wantStatus int
        wantBody   string
    }{
        {
            name:       "existing user",
            userID:     "123",
            wantStatus: http.StatusOK,
            wantBody:   `{"id":"123","name":"Alice"}`,
        },
        {
            name:       "not found",
            userID:     "999",
            wantStatus: http.StatusNotFound,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            req := httptest.NewRequest(http.MethodGet, "/users/"+tt.userID, nil)
            rec := httptest.NewRecorder()

            handler := NewUserHandler(mockStore)
            handler.ServeHTTP(rec, req)

            if rec.Code != tt.wantStatus {
                t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
            }
            if tt.wantBody != "" && strings.TrimSpace(rec.Body.String()) != tt.wantBody {
                t.Errorf("body = %s, want %s", rec.Body.String(), tt.wantBody)
            }
        })
    }
}
```

## Mocking

Mock interfaces, not concrete types. Define interfaces where consumed:

```go
// In the consuming package
type UserStore interface {
    GetUser(ctx context.Context, id string) (*User, error)
    CreateUser(ctx context.Context, u *User) error
}

// Mock implementation for tests
type mockUserStore struct {
    users map[string]*User
    err   error
}

func (m *mockUserStore) GetUser(_ context.Context, id string) (*User, error) {
    if m.err != nil {
        return nil, m.err
    }
    u, ok := m.users[id]
    if !ok {
        return nil, ErrNotFound
    }
    return u, nil
}

func (m *mockUserStore) CreateUser(_ context.Context, u *User) error {
    if m.err != nil {
        return m.err
    }
    m.users[u.ID] = u
    return nil
}
```

### Mock Design Principles

- Keep mocks simple -- just enough to test the behavior
- Use struct fields to control mock behavior (preset errors, data)
- Prefer hand-written mocks over generated ones for simple interfaces
- Use generated mocks (mockgen, moq) for complex interfaces

## Benchmarks

```go
func BenchmarkProcess(b *testing.B) {
    data := generateTestData(1000)

    b.ResetTimer()
    b.ReportAllocs()
    for i := 0; i < b.N; i++ {
        Process(data)
    }
}
```

### Benchmarks with Input Sizes

```go
func BenchmarkSort(b *testing.B) {
    sizes := []int{10, 100, 1000, 10000}
    for _, size := range sizes {
        b.Run(fmt.Sprintf("n=%d", size), func(b *testing.B) {
            data := generateRandomSlice(size)
            b.ResetTimer()
            b.ReportAllocs()
            for i := 0; i < b.N; i++ {
                sorted := make([]int, len(data))
                copy(sorted, data)
                slices.Sort(sorted)
            }
        })
    }
}
```

Use `benchstat` to compare results across runs.

## Fuzzing

Use fuzz tests to find edge cases:

```go
func FuzzParseToken(f *testing.F) {
    // Seed corpus
    f.Add("Bearer abc123")
    f.Add("")
    f.Add("Basic dXNlcjpwYXNz")

    f.Fuzz(func(t *testing.T, input string) {
        token, err := ParseToken(input)
        if err != nil {
            return // invalid input is expected
        }
        // Property: re-serializing should produce valid input
        serialized := token.String()
        reparsed, err := ParseToken(serialized)
        if err != nil {
            t.Errorf("round-trip failed: ParseToken(%q) = %v", serialized, err)
        }
        if token != reparsed {
            t.Errorf("round-trip mismatch: %v != %v", token, reparsed)
        }
    })
}
```

## Integration Tests

Separate with build tags:

```go
//go:build integration

package mypackage_test

import (
    "database/sql"
    "os"
    "testing"
)

func TestDatabaseIntegration(t *testing.T) {
    dsn := os.Getenv("DATABASE_URL")
    if dsn == "" {
        t.Skip("DATABASE_URL not set")
    }

    db, err := sql.Open("postgres", dsn)
    if err != nil {
        t.Fatal(err)
    }
    defer db.Close()

    // Test real database operations
}
```

Run separately: `go test -tags=integration ./...`

## Test Fixtures

Use `testdata/` directory (ignored by Go tooling):

```go
func TestParseConfig(t *testing.T) {
    data, err := os.ReadFile("testdata/valid_config.json")
    if err != nil {
        t.Fatal(err)
    }

    cfg, err := ParseConfig(data)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    // assertions...
}
```

## Goroutine Leak Detection

Use `go.uber.org/goleak` in `TestMain`:

```go
func TestMain(m *testing.M) {
    goleak.VerifyTestMain(m)
}
```

Or per-test:

```go
func TestWorkerPool(t *testing.T) {
    defer goleak.VerifyNone(t)
    // ... test code ...
}
```

## Examples as Documentation

Examples are executable and verified by `go test`:

```go
func ExampleCalculatePrice() {
    price := CalculatePrice(100, 10.0)
    fmt.Printf("Price: %.2f\n", price)
    // Output: Price: 900.00
}
```

## Test Helpers

Mark functions as helpers with `t.Helper()`:

```go
func assertNoError(t *testing.T, err error) {
    t.Helper()
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
}

func assertEqual[T comparable](t *testing.T, got, want T) {
    t.Helper()
    if got != want {
        t.Errorf("got %v, want %v", got, want)
    }
}
```

## File Conventions

```go
// Same package (white-box, access unexported)
package mypackage

// Test package (black-box, public API only)
package mypackage_test
```

Use `_test` package suffix for black-box tests of the public API. Use same package for white-box tests that need access to unexported symbols.

## Sources

- [samber/cc-skills-golang](https://github.com/samber/cc-skills-golang) -- golang-testing skill
- [Uber Go Style Guide](https://github.com/uber-go/guide) -- Test tables, functional options testing
- [Go Blog: Using Subtests and Sub-benchmarks](https://go.dev/blog/subtests)
