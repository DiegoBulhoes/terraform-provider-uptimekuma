# Local demo

A real Uptime Kuma in Docker, managed by the provider built from this repository.

Every resource and data source the provider offers is used here, so this doubles as a working reference.

Nothing is installed or published: Terraform is pointed at the local binary through `dev_overrides`.

## Run it

```bash
make demo
```

That starts the container, creates the admin user, builds the provider and applies everything.

Then open <http://localhost:3001> and log in as `demo` / `demo123`.

The container stays up until you run `make down`.

## Targets

| Target | What it does |
|---|---|
| `make demo` | Everything below, in order. Start here. |
| `make up` | Start Uptime Kuma and create the admin user |
| `make apply` | Build the provider and apply `main.tf` |
| `make plan` | Show what would change |
| `make show` | Print the outputs again |
| `make destroy` | Remove what Terraform created; the container stays up |
| `make down` | Stop the container and delete its data |
| `make logs` | Follow the container logs |
| `make open` | Print the URL and credentials |

Override the defaults with variables: `make demo KUMA_PORT=3005 KUMA_VERSION=2.3.2`.

## What gets created

24 objects: 12 monitors, 2 tags, 2 notification channels, 3 maintenance windows, a proxy, a Docker host, a remote browser, an API key, and the instance settings.

Four monitors go green with no internet access, because they watch the demo instance itself:

- **Uptime Kuma itself** — HTTP against `localhost:3001`
- **Uptime Kuma port** — TCP against port 3001
- **Loopback** — ping to `127.0.0.1`
- **kuma-demo container** — Docker monitor watching the demo container through the mounted socket

The rest point at `example.com`, `api.github.com` and `1.1.1.1`, so they need outbound access.

**Nightly job heartbeat** is a push monitor, and stays pending until something calls it.

The `push_url` output gives you the exact URL:

```bash
TF_CLI_CONFIG_FILE=$PWD/terraformrc terraform output -raw push_url | xargs curl -s
```

## Notes

**`make plan` shows one change, and it is not drift.**

No resource ever differs, but the `available_settings` output grows: Uptime Kuma creates settings keys lazily, so `tlsExpiryNotifyDays` appears once the HTTPS monitors have run.

The output reports what the server actually has, so it follows along.

**`UPTIME_KUMA_DB_TYPE=sqlite` in `docker-compose.yml` is required.**

Without it a fresh Uptime Kuma 2.x stops at a database-selection screen and serves only a stub HTTP listener.

The real API does not exist yet, and Terraform cannot connect.

**The Docker socket is mounted read-only.**

It gives the docker monitor something real to watch.

Remove that volume if you would rather not expose it; the monitor then reports down, and nothing else changes.

**`uptimekuma_settings` writes to the instance.**

It manages only the keys listed in `main.tf` and leaves the rest alone.

`make destroy` does not revert them, because Uptime Kuma has no way to delete a setting, so `make down` is what really cleans up.

**The API key is only shown once.**

Uptime Kuma stores a hash, so the clear-text key exists only in the creation response.

Read it back from state:

```bash
TF_CLI_CONFIG_FILE=$PWD/terraformrc terraform output -raw api_key
```
