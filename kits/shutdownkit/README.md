# ShutdownKit Module

Fx integration for the two-stage shutdown primitives from `signals`.

## Features

- Provides named contexts for graceful and forced shutdown phases.
- Shares a single `*sync.WaitGroup` for managed goroutines.
- Triggers graceful on Fx stop and escalates to force after a timeout (default 10s).
- Helper `shutdownkit.Go` runs background work tied to the shared WaitGroup.
- Timeout override via `shutdownkit.WithTimeout`.

## Usage

```go
type deps struct {
    fx.In
    Graceful context.Context `name:"graceful"`
    Force    context.Context `name:"force"`
    WG       *sync.WaitGroup
}

app := fx.New(
    shutdownkit.Module(shutdownkit.WithTimeout(5*time.Second)),
    fx.Invoke(func(d deps) {
        shutdownkit.Go(d.WG, func() {
            select {
            case <-d.Graceful.Done():
                // flush buffers, close listeners
            case <-d.Force.Done():
                // deadline expired, abort quickly
            }
        })
    }),
)

app.Run()
```

Use `shutdownkit.Module()` together with `signals.NewWithSignals` if you need both Fx lifecycle coordination and OS signal triggering in the same process.
