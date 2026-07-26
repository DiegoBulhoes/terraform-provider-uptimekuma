# Testing

## Requirements

- Go 1.26.5 or newer — the version in `go.mod`. `GOTOOLCHAIN=auto` fetches it for you.
- Docker, for the acceptance tests.
- Terraform on `PATH`, for the acceptance tests and for `tfplugindocs`.

## Layout

| Directory | Build tag | What it covers |
|---|---|---|
| `test/unit/kuma` | — | Wire decoding, error classification, regressions |
| `test/unit/common` | — | Terraform ↔ wire conversions, retry logic |
| `test/unit/provider` | — | Every resource and data source schema |
| `test/unit/resource` | — | Payload building, validation, client call sequences |
| `test/integration/kuma` | `integration` | The Socket.IO client against a live server |
| `test/integration/resource` | `integration` | Resources, through real Terraform plans |
| `test/integration/datasource` | `integration` | Data sources, through real Terraform plans |

Unit tests come in three flavors, each in its own file:

- **`happy_test.go`** — the shapes and values a working setup produces.
- **`sad_test.go`** — the server said no, or the connection did. Each failure has to land in the right bucket, because the bucket decides whether the provider retries, drops the object from state, or gives up.
- **`absurd_test.go`** — payloads no healthy server would send, and values no sane user would type. This is where the raw-JSON attributes get stress-tested.

## Running the tests

Unit tests only. Fast, no Docker:

```bash
go test -race ./test/unit/...
```

Everything CI checks, without changing files:

```bash
make ci
```

Full acceptance suite against every supported version:

```bash
make ci-acceptance          # 2.2.1, 2.3.2, 2.4.0
```

One version:

```bash
UPTIME_KUMA_IMAGE=louislam/uptime-kuma:2.4.0 TF_ACC=1 \
  TF_ACC_TERRAFORM_PATH=$(which terraform) \
  go test -p 1 -tags integration -timeout 900s -count=1 ./test/integration/...
```

`-p 1` matters: each acceptance package starts its own container, and three at once starve each other into timeouts.

## Iterating quickly

Starting a container per package costs about a minute.

While working on one resource, keep an instance running and point the tests at it:

```bash
docker run -d --name kuma-dev -p 3001:3001 -e UPTIME_KUMA_DB_TYPE=sqlite louislam/uptime-kuma:2.4.0

UPTIME_KUMA_URL=http://localhost:3001 TF_ACC=1 \
  TF_ACC_TERRAFORM_PATH=$(which terraform) \
  go test -tags integration -count=1 -run TestAccMonitorHTTP ./test/integration/resource/
```

The test setup creates the admin account either way, whether it started the instance or you did.

**`UPTIME_KUMA_DB_TYPE=sqlite` is not optional.**

A fresh Uptime Kuma 2.x boots into a database-selection step and serves only a stub HTTP listener until you answer it.

The real Socket.IO API does not exist yet, so without that variable the tests hang waiting for a handshake that never comes.

State left over from an earlier run makes tests fail in confusing ways: duplicate names, or list counts that do not add up.

Recreate the container when that happens:

```bash
docker rm -f kuma-dev
```

The demo environment in `examples/demo` is another way to get a working instance, with a Makefile around it.

## How the acceptance setup works

`test/acctest/acctest.go`:

1. Uses `UPTIME_KUMA_URL` if it is set. Otherwise starts a container with `UPTIME_KUMA_DB_TYPE=sqlite`.

2. Waits for `/socket.io/?EIO=4&transport=polling`, not the HTTP root. The setup-database stub answers the root, so waiting on it would move on too early.

3. Calls `needSetup` and, if needed, `setup` to create the admin account. `needSetup` makes this safe to repeat.

4. Exports `UPTIME_KUMA_URL`, `UPTIME_KUMA_USERNAME` and `UPTIME_KUMA_PASSWORD` for the tests and for the provider block that `acctest.ProviderConfig()` builds.

The provider runs in-process through `acctest.ProviderFactories()`, so nothing has to be built or installed.

## Debugging a failure

**Start with `test/integration/kuma`.**

It drives the client directly, which separates a client problem from a resource problem:

```bash
UPTIME_KUMA_URL=http://localhost:3001 go test -tags integration -v -run TestClientLifecycle ./test/integration/kuma/
```

**"Too frequently, try again later."**

That is the server's login limiter: 20 per minute for the whole instance.

Wait a minute, or raise `max_retries`. Inside one process the client already shares a session per configuration.

**A hang in the transport, or flaky failures with no message.**

The Socket.IO library has reported deadlocks in channel sends and a race on its namespace map.

Try the patched fork before you suspect this codebase:

```
replace github.com/maldikhan/go.socket.io => github.com/breml/go.socket.io v0.0.0-20260516193936-e70410c8cd31
```

**An object that "does not exist" right after being created.**

Almost always a decoding failure in a pushed list.

Those are decoded inside event handlers, where no error can be returned, so the cache just stays empty.

Check the JSON tags, and whether a boolean field should be `kuma.Bool`.

**"Provider produced inconsistent result after apply".**

Either an attribute the server computes is not being read back, or a value the server normalizes is not marked `Computed`.

Compare what `getMonitor` returns against what the model writes.

## Coverage

**The project requires combined coverage above 95%.** `make coverage` measures it and fails below that. CI has a job that does the same.

```bash
make coverage        # both suites, merged, then checked against the minimum
make coverage-gaps   # the 30 least covered functions, to pick the next test
```

Combined is the only number that means anything here. Neither suite reaches the threshold alone, and neither is supposed to:

- The **acceptance tests** cover the wire format. They are the only thing that proves a payload is one the server accepts.

- The **unit tests** cover what a healthy server will not produce on demand: an acknowledgement with no ID, a rejected session token, a state file that will not decode, a push that never arrives.

`go tool covdata` merges the two, which is why `make coverage` writes to `-test.gocoverdir` instead of collecting a text profile per suite.

### Writing for the number, and not

Chasing the last few points is how coverage stops being useful.

Two rules kept it honest here:

- Prefer tests that walk a registry over tests that name one resource. `TestEveryOperationStopsOnAnUndecodableModel` covers a guard in every CRUD method of every resource, and covers the next resource for free.

- If a test can only be written by asserting that Go propagates an error, say what the guard prevents instead. If nothing can be said, the guard may be what needs changing, not the test.

Writing tests for the number did find real bugs: a panic on a nil base context, `null` accepted as a settings document, and an empty slug reaching the server as a request for the incidents of no page at all.

Every one of them came from a test covering real logic, and none from a test covering an `if err != nil`. That is worth remembering when picking the next gap to close.

### Regression tests

`test/unit/kuma/regression_test.go` and `basecontext_test.go` hold the tests for bugs this project has already had.

They are grouped there because none of them belongs to a single method — each is a property of the package.

Each names the failure it prevents rather than the code it covers, so reintroducing the bug produces a message that explains it.

Check that when adding one: break the fix on purpose and read what the test says.
