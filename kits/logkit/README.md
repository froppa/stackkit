# Logkit Module

Zap logger integration for Fx applications.

## Features

- Provides both `*zap.Logger` and `*zap.SugaredLogger`.
- Configurable encoding (`production`, `json`, `development`).
- Configurable log level.
- Logs service metadata (via `runtimeinfo`) on startup.
- Flushes buffered logs on shutdown.

## Config

```yaml
logger:
  encoding: production  # or development/json
  level: info           # debug|info|warn|error
```

Validation enforces that `encoding` is one of `production|prod|json|development|dev|console` and that `level` resolves to a valid Zap level. Startup fails if the values are out of range.

## Usage

```go
app := fx.New(
  logkit.Module(),
  fx.Invoke(func(log *zap.Logger) {
    log.Info("hello")
  }),
)
```
