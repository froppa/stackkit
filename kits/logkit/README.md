# Logkit Module

Zap logger integration for Fx applications.

## Features

- Provides both `*zap.Logger` and `*zap.SugaredLogger`.
- Configurable encoding (`production`, `json`, `development`).
- Configurable log level.
- Logs service metadata (via `runtimeinfo`) on startup.
- Flushes buffered logs on shutdown.

## Config

`Module()` defaults to production JSON at info level. It does not load YAML.
Override its configuration with `fx.Replace`:

```go
logkit.Module(),
fx.Replace(logkit.Config{Encoding: "development", Level: "debug"}),
```

`New` rejects unknown encodings and invalid Zap levels.

## Usage

```go
app := fx.New(
  logkit.Module(),
  fx.Invoke(func(log *zap.Logger) {
    log.Info("hello")
  }),
)
```
