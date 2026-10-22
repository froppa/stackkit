# HTTPKit Module

HTTP server integration for Fx applications.

## Features

- Provides `net.Listener` bound to configured address.
- Provides `*http.ServeMux`.
- Opt-in `/debug/pprof` endpoints.
- Supports grouped route registration (`group:"http.handlers"`).
- Graceful shutdown with Fx lifecycle.

## Config

```yaml
http:
  addr: ":8080"
  read_timeout_ms: 5000
  write_timeout_ms: 5000
  enable_pprof: false
```

`httpkit.Config` uses `validate` tags, so `addr` must be provided and timeout values must be non-negative. Invalid configs fail fast when the Fx app starts.

## Usage

See the [runnable service example](../../examples/service-basic/main.go) for
configuration and logging setup. Register routes as grouped `httpkit.Handler`
values:

```go
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
```
