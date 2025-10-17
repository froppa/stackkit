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

```go
app := fx.New(
  httpkit.Module(),
  fx.Provide(func() http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
      w.Write([]byte("pong"))
    })
  }),
  fx.Provide(func(h http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
      h.ServeHTTP(w, r)
    })
  }),
)
```
