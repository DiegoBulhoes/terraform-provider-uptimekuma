# CLAUDE.md

## Project Overview

Terraform provider for Uptime Kuma 2.x, built with `terraform-plugin-framework`.

It manages monitors (one resource per monitor type), tags, notifications, maintenance windows, proxies, Docker hosts, remote browsers, API keys and settings.

## Quick Commands

```bash
make all          # Full CI: tidy, lint, security, test, build, docs
make build        # Compile provider binary
make test         # Unit tests + acceptance tests (Uptime Kuma 2.2-2.4)
make lint         # fmt + vet + golangci-lint
make docs         # Generate and validate docs with tfplugindocs
make fmt          # gofmt + goimports
make security     # govulncheck
make ci           # The CI checks and unit tests, without changing files
make coverage     # Both suites merged; fails below 95%
make coverage-gaps # The least covered functions, to pick the next test
```

Run a single unit test:
```bash
go test -run TestFunctionName ./test/unit/kuma/...
```

Acceptance tests against one Uptime Kuma version:
```bash
UPTIME_KUMA_IMAGE=louislam/uptime-kuma:2.4.0 TF_ACC=1 \
  TF_ACC_TERRAFORM_PATH=$(which terraform) \
  go test -p 1 -tags integration -timeout 900s -count=1 ./test/integration/...
```

Against an instance that is already running, which is much faster while iterating:
```bash
docker run -d -p 3001:3001 -e UPTIME_KUMA_DB_TYPE=sqlite louislam/uptime-kuma:2.4.0
UPTIME_KUMA_URL=http://localhost:3001 TF_ACC=1 \
  TF_ACC_TERRAFORM_PATH=$(which terraform) \
  go test -tags integration -count=1 ./test/integration/...
```

A full local environment, container included:
```bash
cd examples/demo && make demo
```

## Architecture

```
internal/
  kuma/         # Socket.IO client: connection, RPC, push cache, wire types
  common/       # KumaClient interface (mockable) + Terraform<->wire helpers
  provider/     # Provider config, delegates registration to the aggregators
  resource/     # resource.go lists them; one subpackage per resource
    monitor/    #   the shared CRUD cycle plus one file per monitor type
    tag/ notification/ maintenance/
    proxy/ dockerhost/ remotebrowser/ apikey/ settings/
  datasource/   # Same layout, mirroring the resources

test/
  acctest/      # Acceptance infra: testcontainers + admin bootstrap
  mocks/        # Generated GoMock mock of common.KumaClient
  unit/         # happy_test.go / sad_test.go / absurd_test.go per package
  integration/  # Acceptance tests (kuma, resource, datasource)
tools/
  kuma-bootstrap/  # Creates the first admin user on a fresh instance
```

Each resource is its own package so a domain stays self-contained. The `monitor` package covers 9 of Uptime Kuma's 33 monitor types so far.

`internal/resource/resource.go` and `internal/datasource/datasource.go` list everything, so the provider imports two packages instead of twenty.

## The API this provider talks to

**Uptime Kuma has no REST API for writes.** Everything is Socket.IO events.

Every one of these was verified against the upstream source, and each shaped the code:

1. **Creating a monitor is the `add` event**, not `addMonitor` (`server/server.js`).

2. **`editMonitor` never writes the `active` column.** It assigns every other field one by one and leaves that one alone, so pausing and resuming go through `pauseMonitor` and `resumeMonitor`. Sending `active` in an update payload does nothing.

3. **`accepted_statuscodes` is dereferenced without a nil check**, and every entry has to be a string. The client injects `["200-299"]` when it is absent.

4. **`retryInterval` has no server-side default and zero is rejected.** The client mirrors the check interval, as the web UI does.

5. **`notificationIDList` is an object, not an array** (`{"1": true}`), and an empty object is how links get removed — so the field must never be omitted.

6. **The push token is generated client-side.** The server never creates one; the web UI does, and so does this client.

7. **Booleans arrive as 0 or 1.** Payloads built from database rows (proxies, API keys) are dumped straight to JSON, and SQLite has no boolean type. `kuma.Bool` absorbs that. Without it the pushed lists fail to decode *silently*, and every object looks like it does not exist.

8. **Several entities have no getter event.** Notifications, proxies, Docker hosts and remote browsers only ever arrive by push, so `internal/kuma/cache.go` keeps the lists. `getMonitorList`, `getMaintenanceList` and `getAPIKeyList` acknowledge with a bare `{ok:true}` and deliver the payload on the push channel too.

9. **Logins are limited to 20 per minute server-wide.** `internal/kuma/pool.go` shares one session per configuration inside a process, and rate-limit rejections get a slower backoff than other retries.

10. **`maintenance.dateRange` is always indexed by the server**, whatever the strategy, and `active` is NOT NULL with no default. `NormalizeMaintenance` fills both in.

11. **Optional fields need `omitempty` for compatibility.** On create the server feeds the payload to `bean.import()`, which turns each key into a column in the INSERT. Sending `"bearer_token": null` to a version that predates that column fails the whole statement. Omitting absent fields keeps one payload working across 2.2 to 2.4. The exception is a field a user can clear — a proxy's credentials, an API key's expiry — where null is the only way to empty it, and the column has existed since 1.x anyway. `TestOptionalFieldsAreOmittedForTheOtherEntities` holds that list.

12. **Updates are built from the plan, not merged onto the server's copy.** Merging looks safer but makes removing an attribute impossible: a deleted value arrives as null, leaves the wire struct untouched, and the old value gets written straight back.

13. **A fresh 2.x instance stops at a database-selection step**, with only a stub HTTP server. `UPTIME_KUMA_DB_TYPE=sqlite` skips it, and the acceptance containers set it.

14. **Not-found has no distinct response.** A missing row makes the server dereference null and report a JavaScript TypeError, so `APIError.Is` matches on the message text.

## Adding a monitor type

`internal/resource/monitor/resource.go` handles the whole CRUD cycle. A new type needs:

1. A model embedding `monitor.BaseModel`, plus `monitor.HTTPBase` for the HTTP-based types.

2. `Base()`, `ApplyTo()` and `ReadFrom()`. The last two only handle the type-specific fields.

3. A `NewXResource()` returning `New(TypeDef{...})`.

4. An entry in `internal/resource/resource.go`.

5. An acceptance test, plus `examples/resources/<name>/{resource.tf,import.sh}`.

Wire field names come from `server/model/monitor.js` (`toJSON`). **That format mixes snake_case and camelCase in the same payload** — do not normalize the JSON tags, map them field by field.

## Testing Conventions

- **Unit tests** (`test/unit/`) live in external `_test` packages and use GoMock over `common.KumaClient`. Do not combine `t.Setenv` with `t.Parallel`; the testing package forbids it.

- Each package splits its unit tests into `happy_test.go`, `sad_test.go` and `absurd_test.go`. The absurd ones matter more than usual here: `settings` and `notification` take raw JSON from the user, and pushed lists are decoded where no error can be returned.

- **Acceptance tests** (`test/integration/`) are behind `//go:build integration` and use real containers. Run them with `-p 1`: each package starts its own container, and three at once starve each other into timeouts.

- `test/integration/kuma` exercises the client directly, and is the first thing to run when something breaks.

- Mock generation: `go tool mockgen -destination=test/mocks/mock_client.go -package=mocks github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/common KumaClient`

- Several tests walk a registry (`resource.All()`, `provider.Resources()`) instead of naming resources one by one, so a resource added later is covered without anyone remembering to add it. `TestEveryMonitorTypeRoundTripsItsAttributes` is the important one: it drives every type's `ApplyTo`/`ReadFrom` pair and fails when the two disagree about an attribute, which is otherwise a permanent diff no compiler catches.

- **Combined coverage must stay above 95%**, measured by `make coverage`, which merges the two suites with `go tool covdata`. Neither suite reaches it alone and neither should: acceptance tests cover the wire format, unit tests cover what a healthy server will not produce on demand. When adding a test for the number, prefer one that walks a registry over one that names a single resource — see `TESTING.md`.

- Regression tests live in `test/unit/kuma/regression_test.go` and `basecontext_test.go`. Each names the failure it prevents, so reintroducing the bug produces a message that explains it.

- **A panic is worse than an error here**: it kills the plugin process, and the framework reports a crash with no resource address while an operation is left half-applied. Cover the root cause, not just the guard.

## Code Style

- Go 1.26+ with `terraform-plugin-framework`, not SDKv2.

- All server access goes through `common.KumaClient` so it can be mocked.

- Transport is `github.com/maldikhan/go.socket.io`. **Its ack callbacks have to be `func([]any)`.** Any other signature takes the library's reflection path, which requires every argument to be a `json.RawMessage` and silently drops the callback when it is not. That is how the connect handler fails.

- Known issue: that library has reported deadlocks in channel sends and a race on its namespace map. If tests hang or flake in the transport, apply `replace github.com/maldikhan/go.socket.io => github.com/breml/go.socket.io v0.0.0-20260516193936-e70410c8cd31` before suspecting this codebase.

- The library's default logger writes to stdout, which would corrupt the plugin protocol. `internal/kuma/logger.go` redirects everything to tflog.

- Linter config: `.golangci.yml`.

## Documentation

- **Do not edit** files in `docs/`. Edit the templates in `templates/` and the examples in `examples/`, then run `make docs`.

- `tfplugindocs` needs `--provider-name uptimekuma`, because it otherwise infers the name from the directory. The Makefile passes it.

- The repository should be named `terraform-provider-uptimekuma`: both the Registry and `tfplugindocs` derive the provider name from it, and `uptime-kuma` would give resources a hyphen.
