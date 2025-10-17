# stackkit

Personal Go modules for wiring microservices with [Uber Fx](https://github.com/uber-go/fx).  
Everything here is designed to be drop-in, explicit, and easy to reason about when building small services or tools.

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

```go
package main

import (
	"context"
	"net/http"

	"github.com/froppa/stackkit/kits/configkit"
	"github.com/froppa/stackkit/kits/fxeventlog"
	"github.com/froppa/stackkit/kits/healthkit"
	"github.com/froppa/stackkit/kits/httpkit"
	"github.com/froppa/stackkit/kits/logkit"
	"github.com/froppa/stackkit/kits/shutdownkit"
	"github.com/froppa/stackkit/kits/telemetry"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"
)

func main() {
	app := fx.New(
		fx.WithLogger(func(log *zap.Logger) fxevent.Logger {
			return fxeventlog.NewMinimal(log)
		}),
		fx.Supply(context.Background()),
		configkit.Module(
			configkit.WithEmbeddedBytes([]byte(`
http:
  addr: ":8080"
health: {}
telemetry:
  disabled: true
`)),
		),
		logkit.Module(),
		httpkit.Module(),
		healthkit.ServerModule(),
		telemetry.Module(),
		shutdownkit.Module(),
		fx.Provide(fx.Annotate(
			func() httpkit.Handler {
				return httpkit.Handler{
					Pattern: "/ping",
					Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						_, _ = w.Write([]byte("pong"))
					}),
				}
			},
			fx.ResultTags(`group:"http.handlers"`),
		)),
	)

	app.Run()
}
```

Configuration lives under `./config` by default.

1. Embed defaults via `configkit.WithEmbeddedBytes`.
2. Override with `config/config.yml`, `config/config.local.yml`, and `config/<service>.yml` (uses `runtimeinfo.Name`).
3. Expand `${VAR:default}` placeholders at load time.

CLI helpers:

- `go run github.com/froppa/stackkit/cmd/stackctl config check --all`
- `go run github.com/froppa/stackkit/cmd/stackctl config discovery --from-yaml=./config/config.yml`
- `go run github.com/froppa/stackkit/cmd/stackctl config list --key=http --config=./config/config.yml`

Bring your own Fx modules around these pieces; everything here is intentionally small and composable.
