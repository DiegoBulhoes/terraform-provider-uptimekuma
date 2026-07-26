# Go Security Review Checklist

Full security review checklist organized by domain.

## Input Handling

- [ ] All user input is validated at trust boundaries
- [ ] Input length limits are enforced
- [ ] Allow-listing is preferred over deny-listing
- [ ] Path traversal is prevented (`os.Root` Go 1.24+, `filepath.Clean`, base path validation)
- [ ] File upload size limits are enforced
- [ ] Content-Type is validated for file uploads

## SQL and Database

- [ ] All queries use parameterized statements (`database/sql` with `?` placeholders)
- [ ] NEVER string concatenation for SQL queries
- [ ] Database connections use least-privilege credentials
- [ ] Connection pools have reasonable limits
- [ ] Prepared statements are used for repeated queries

```go
// Good -- parameterized
row := db.QueryRowContext(ctx, "SELECT name FROM users WHERE id = $1", id)

// Bad -- SQL injection
row := db.QueryRowContext(ctx, "SELECT name FROM users WHERE id = "+id)
```

## Command Execution

- [ ] `exec.Command` uses separate arguments, NEVER shell concatenation
- [ ] User input NEVER reaches command arguments without validation
- [ ] `bash -c` is NEVER used with user-controlled strings

```go
// Good -- separate arguments
cmd := exec.Command("convert", inputFile, "-resize", "100x100", outputFile)

// Bad -- shell injection
cmd := exec.Command("bash", "-c", "convert "+inputFile+" -resize 100x100 "+outputFile)
```

## Cryptography

- [ ] `crypto/rand` for all security-sensitive randomness (tokens, keys, nonces)
- [ ] NEVER `math/rand` for security purposes
- [ ] AES-GCM for symmetric encryption (encrypt + authenticate)
- [ ] Argon2id or bcrypt for password hashing (NEVER MD5/SHA1)
- [ ] TLS 1.2+ enforced for all external connections
- [ ] Certificate validation is NOT disabled in production

```go
// Good -- crypto/rand for tokens
token := make([]byte, 32)
if _, err := crypto_rand.Read(token); err != nil {
    return err
}

// Good -- argon2id for passwords
hash := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)
```

## Web and HTTP

- [ ] `html/template` for HTML output (auto-escaping)
- [ ] NEVER `text/template` for user-facing HTML
- [ ] Security headers set: HSTS, X-Content-Type-Options, X-Frame-Options, CSP
- [ ] CORS configured with specific origins (not `*` in production)
- [ ] Server timeouts configured (Read, Write, Idle)
- [ ] Request body size limits enforced
- [ ] SSRF prevention: validate/allow-list URLs before fetching

```go
srv := &http.Server{
    Addr:         ":8080",
    ReadTimeout:  5 * time.Second,
    WriteTimeout: 10 * time.Second,
    IdleTimeout:  120 * time.Second,
    Handler:      handler,
}
```

## Authentication and Authorization

- [ ] Authentication on ALL endpoints (no security through obscurity)
- [ ] Authorization checks server-side (NEVER trust client-side checks)
- [ ] Session tokens are cryptographically random
- [ ] Cookies: `Secure`, `HttpOnly`, `SameSite=Strict` flags set
- [ ] JWT validation includes expiry, issuer, audience checks
- [ ] Password comparison uses `crypto/subtle.ConstantTimeCompare`

```go
// Good -- constant-time comparison
if subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
    return ErrUnauthorized
}

// Bad -- timing attack vulnerable
if provided != expected {
    return ErrUnauthorized
}
```

## Secrets Management

- [ ] NO hardcoded secrets in source code
- [ ] Secrets loaded from environment variables or secret managers
- [ ] Different secrets per environment (dev/staging/prod)
- [ ] `.env` files in `.gitignore`
- [ ] Secrets NOT logged (even at debug level)

## Error Handling and Logging

- [ ] Internal errors NOT exposed to API consumers
- [ ] Generic error messages for users, detailed logs server-side
- [ ] No PII in log messages
- [ ] No log injection (sanitize user input before logging)
- [ ] Stack traces NOT returned in API responses

```go
// Good -- generic message to user, details logged
slog.Error("database query failed", "error", err, "query", "GetUser", "user_id", id)
http.Error(w, "internal server error", http.StatusInternalServerError)

// Bad -- leaks internals
http.Error(w, fmt.Sprintf("database error: %v", err), http.StatusInternalServerError)
```

## Concurrency Safety

- [ ] Shared state protected by `sync.Mutex` or channels
- [ ] `go test -race ./...` in CI pipeline
- [ ] Concurrent map access uses `sync.Mutex` or `sync.Map` (concurrent read/write causes hard crash)
- [ ] All race detector findings are fixed

## Dependencies

- [ ] `govulncheck ./...` run regularly
- [ ] `go.sum` committed to version control
- [ ] Dependencies reviewed before adoption
- [ ] Minimal dependency tree (fewer dependencies = smaller attack surface)

## Rate Limiting and DoS Prevention

- [ ] Rate limiting on authentication endpoints
- [ ] Request body size limits (`http.MaxBytesReader`)
- [ ] Timeouts on all I/O operations
- [ ] Connection limits configured
- [ ] Zip bomb protection for archive handling

```go
// Limit request body size
r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB limit
```

## Tooling

```bash
# Static security analysis
gosec ./...

# Vulnerability scanning
govulncheck ./...

# Race detection
go test -race ./...

# Fuzz testing for edge cases
go test -fuzz=Fuzz ./...

# Linting with security rules
golangci-lint run --enable gosec,bodyclose,sqlclosecheck,nilerr
```

## Severity Reference (DREAD)

| Level | Score | Meaning | Response |
|-------|-------|---------|----------|
| Critical | 8-10 | RCE, data breach, credential theft | Fix immediately |
| High | 6-7.9 | Auth bypass, data exposure, broken crypto | Fix in current sprint |
| Medium | 4-5.9 | Limited exposure, session issues | Fix in next sprint |
| Low | 1-3.9 | Minor disclosure, best-practice deviations | Fix opportunistically |

## Sources

- [samber/cc-skills-golang](https://github.com/samber/cc-skills-golang) -- golang-security skill
- [Uber Go Style Guide](https://github.com/uber-go/guide) -- Avoid mutable globals, atomic operations
- [OWASP Go Secure Coding Practices](https://owasp.org/www-project-go-secure-coding-practices-guide/)
- [Go Security Best Practices](https://go.dev/doc/security/best-practices)
