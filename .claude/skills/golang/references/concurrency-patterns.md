# Go Concurrency Patterns

Detailed concurrency patterns and sync primitive usage.

## errgroup -- The Go-To for Goroutine Groups

`errgroup` replaces most hand-rolled WaitGroup + error patterns:

```go
import "golang.org/x/sync/errgroup"

func fetchAll(ctx context.Context, urls []string) ([]Response, error) {
    g, ctx := errgroup.WithContext(ctx)
    responses := make([]Response, len(urls))

    for i, url := range urls {
        g.Go(func() error {
            resp, err := fetch(ctx, url)
            if err != nil {
                return fmt.Errorf("fetching %s: %w", url, err)
            }
            responses[i] = resp  // safe: each goroutine writes to its own index
            return nil
        })
    }

    if err := g.Wait(); err != nil {
        return nil, err
    }
    return responses, nil
}
```

### With Concurrency Limit

```go
g, ctx := errgroup.WithContext(ctx)
g.SetLimit(10)  // max 10 concurrent goroutines

for _, item := range items {
    g.Go(func() error {
        return process(ctx, item)
    })
}
```

## Worker Pool Pattern

For more control than errgroup:

```go
func workerPool(ctx context.Context, jobs <-chan Job, workers int) <-chan Result {
    results := make(chan Result)
    var wg sync.WaitGroup

    for range workers {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for job := range jobs {
                select {
                case <-ctx.Done():
                    return
                case results <- process(job):
                }
            }
        }()
    }

    go func() {
        wg.Wait()
        close(results)
    }()

    return results
}
```

## Fan-Out / Fan-In Pipeline

```go
func pipeline(ctx context.Context, input <-chan Data) <-chan Result {
    // Stage 1: Transform (fan-out to N workers)
    transformed := make(chan Intermediate)
    var wg sync.WaitGroup
    for range runtime.NumCPU() {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for data := range input {
                select {
                case <-ctx.Done():
                    return
                case transformed <- transform(data):
                }
            }
        }()
    }
    go func() {
        wg.Wait()
        close(transformed)
    }()

    // Stage 2: Aggregate (fan-in)
    results := make(chan Result)
    go func() {
        defer close(results)
        for item := range transformed {
            select {
            case <-ctx.Done():
                return
            case results <- aggregate(item):
            }
        }
    }()

    return results
}
```

## Sync Primitives

### sync.Mutex -- Protecting Shared State

```go
type SafeCounter struct {
    mu sync.Mutex
    v  map[string]int
}

func (c *SafeCounter) Inc(key string) {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.v[key]++
}

func (c *SafeCounter) Value(key string) int {
    c.mu.Lock()
    defer c.mu.Unlock()
    return c.v[key]
}
```

Rules:
- Keep critical sections short -- NEVER hold across I/O
- NEVER embed mutexes in public structs -- use a named field
- Zero-value is valid -- no need for `new(sync.Mutex)`
- NEVER upgrade `RLock` to `Lock` (deadlock)

### sync.Once -- One-Time Initialization

```go
var (
    instance *Database
    once     sync.Once
)

func GetDB() *Database {
    once.Do(func() {
        instance = connectDB()
    })
    return instance
}
```

Go 1.21+: `sync.OnceFunc`, `sync.OnceValue`, `sync.OnceValues` for cleaner patterns.

### sync.Pool -- Object Reuse

```go
var bufPool = sync.Pool{
    New: func() any {
        return new(bytes.Buffer)
    },
}

func process(data []byte) string {
    buf := bufPool.Get().(*bytes.Buffer)
    defer func() {
        buf.Reset()  // MUST reset before returning to pool
        bufPool.Put(buf)
    }()

    buf.Write(data)
    return buf.String()
}
```

### singleflight -- Deduplicating Concurrent Calls

Prevents cache stampede:

```go
import "golang.org/x/sync/singleflight"

var group singleflight.Group

func getUser(ctx context.Context, id string) (*User, error) {
    v, err, _ := group.Do(id, func() (any, error) {
        return db.GetUser(ctx, id)
    })
    if err != nil {
        return nil, err
    }
    return v.(*User), nil
}
```

## Channel Patterns

### Done Channel for Shutdown

```go
func worker(done <-chan struct{}, jobs <-chan Job) {
    for {
        select {
        case <-done:
            return
        case job, ok := <-jobs:
            if !ok {
                return
            }
            process(job)
        }
    }
}
```

### Context Cancellation in Select

ALWAYS include `ctx.Done()`:

```go
func fetch(ctx context.Context, url string) (*Response, error) {
    ch := make(chan *Response, 1)
    go func() {
        ch <- doFetch(url)
    }()

    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    case resp := <-ch:
        return resp, nil
    }
}
```

### Timer in Loops

NEVER use `time.After` in loops -- it leaks timers:

```go
// Good -- reuse timer
timer := time.NewTimer(timeout)
defer timer.Stop()

for {
    timer.Reset(timeout)
    select {
    case <-timer.C:
        handleTimeout()
    case msg := <-ch:
        handleMessage(msg)
    }
}

// Bad -- each iteration creates a new timer that lives until it fires
for {
    select {
    case <-time.After(timeout):  // LEAK
        handleTimeout()
    case msg := <-ch:
        handleMessage(msg)
    }
}
```

## Graceful Shutdown

```go
func main() {
    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()

    srv := &http.Server{Addr: ":8080", Handler: handler}

    go func() {
        if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            slog.Error("server error", "error", err)
        }
    }()

    <-ctx.Done()
    slog.Info("shutting down")

    shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    if err := srv.Shutdown(shutdownCtx); err != nil {
        slog.Error("shutdown error", "error", err)
    }
}
```

## Common Mistakes

| Mistake | Fix |
|---------|-----|
| Fire-and-forget goroutine | Provide stop mechanism (context, done channel) |
| Closing channel from receiver | Only the sender closes |
| `time.After` in hot loop | Reuse `time.NewTimer` + `Reset` |
| Missing `ctx.Done()` in select | Always select on context |
| Unbounded goroutine spawning | `errgroup.SetLimit(n)` or semaphore |
| Sharing pointer via channel | Send copies or immutable values |
| `wg.Add` inside goroutine | Call `Add` before `go` |
| Mutex held across I/O | Keep critical sections short |
| Concurrent map read/write | Use `sync.Mutex` or `sync.Map` (hard crash, not race) |

## Sources

- [samber/cc-skills-golang](https://github.com/samber/cc-skills-golang) -- golang-concurrency skill
- [Uber Go Style Guide](https://github.com/uber-go/guide) -- Channel size, goroutine lifecycle, atomic
- [Go Concurrency Patterns: Pipelines](https://go.dev/blog/pipelines) -- Pipeline patterns
