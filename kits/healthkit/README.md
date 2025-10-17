# HealthKit Module

Liveness and readiness reporting for Fx applications.
Designed for use with orchestrators and load balancers (e.g. Kubernetes).

## Modes

1. **Dedicated server (ServerModule)**
   Starts its own HTTP server on a configurable port.
   Recommended for isolation from application traffic.

2. **Mux attachment (MuxModule)**
   Registers `/health` on an existing `*http.ServeMux`.
   Useful if the app already exposes HTTP.

## Config

```yaml
    health:
      port: ":8081"          # only used with ServerModule()
      startup_delay: 200ms   # wait before marking ready
```

## Responses

- `200 OK` when live and ready.
- `503 Service Unavailable` with `{"status":"initializing"}` until ready.
- `503 Service Unavailable` with `{"status":"unhealthy"}` after stop.

## Usage

Dedicated server:

```go
    app := fx.New(
      healthkit.ServerModule(),
    )
```

Mux attachment:

```go
    app := fx.New(
      httpkit.Module(),
      healthkit.MuxModule(),
    )
```
