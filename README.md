# stackkit

[![CI](https://github.com/froppa/stackkit/actions/workflows/test.yml/badge.svg)](https://github.com/froppa/stackkit/actions/workflows/test.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/froppa/stackkit.svg)](https://pkg.go.dev/github.com/froppa/stackkit)

Go packages I use to wire services with [Uber Fx](https://github.com/uber-go/fx):
configuration, logging, HTTP, health checks, telemetry, and shutdown.
The kits share a few conventions but can be used individually.

Requires Go 1.24 or later. The API is still evolving.

```sh
go get github.com/froppa/stackkit
```

## Layout

- `kits/*` — reusable Fx kits (configkit, httpkit, telemetry, etc.)
- `cmd/stackctl` — Cobra CLI for working with config discovery/list/check.
- `examples/` — runnable skeletons (e.g. `service-basic` wires HTTP + telemetry).
- `README.md`, `Makefile`, `.github/` — project docs, tooling, CI.

## Kits

- `configkit` — layered YAML loader with validation and discovery helpers.
- `logkit` — opinionated `zap.Logger` setup plus a calmer Fx event logger (`fxeventlog`).
- `httpkit` — HTTP server module with grouped handlers, graceful shutdown, and optional `pprof`.
- `healthkit` — readiness/liveness reporting, either on a dedicated port or an existing mux.
- `telemetry` — OpenTelemetry traces + metrics, driven by config and `runtimeinfo`.
- `runtimeinfo` — build metadata helpers for logging and observability labels.
- `signals` — graceful/forced shutdown coordination.
- `shutdownkit` — Fx integration for `signals`, exporting named contexts and a shared `WaitGroup`.

## Quick Start

Run the [basic service](examples/service-basic/main.go) from a checkout:

```sh
go run ./examples/service-basic
```

It serves `/ping` and `/health` on port 8080, with telemetry export disabled.
In another terminal:

```sh
curl http://localhost:8080/ping
curl http://localhost:8080/health
```

The example shows the complete Fx wiring, including grouped HTTP handlers.
Use Ctrl-C to stop the service.

Configuration lives under `./config` by default.

1. Embed defaults via `configkit.WithEmbeddedBytes`.
2. Override with `config/config.yml`, `config/config.local.yml`, and `config/<service>.yml` (uses `runtimeinfo.Name`).
3. Expand `${VAR:default}` placeholders at load time.

The CLI uses a separate precedence: `config/config.yml`, then `$CONFIG`, then
`--config`. It registers HTTP and telemetry configuration types.

CLI helpers (from a checkout):

- `go run ./cmd/stackctl config check --all --config=examples/service-basic/config.yml`
- `go run ./cmd/stackctl config discovery --from-yaml=examples/service-basic/config.yml`
- `go run ./cmd/stackctl config list --key=http --config=examples/service-basic/config.yml`

## Development

With [mise](https://mise.jdx.dev/) installed:

```sh
mise install
mise exec -- make all
```

`make all` runs vet, lint, and race-enabled tests. `make fmt` and `make tidy`
are explicit maintenance commands. `make tools` installs the pinned linter
if you manage Go yourself.

## License

[MIT](LICENSE).
