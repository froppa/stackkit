# Signals Module

Graceful and forced shutdown orchestration.

## Features

- Handles `SIGINT` and `SIGTERM` for CLI tools or services.
- Provides two contexts:
  - **Graceful**: cancelled on first signal.
  - **Force**: cancelled when the timeout in `Wait` elapses.
- WaitGroup support for in-flight goroutines.

## Usage

```go
wg := &sync.WaitGroup{}
s := signals.NewWithSignals(context.Background(), wg)

wg.Add(1)
go func() {
    defer wg.Done()
    <-s.Graceful().Done() // cleanup
}()

s.Wait(10 * time.Second) // blocks until done or force timeout
```

`Wait` still waits for workers to finish after cancelling the force context.
Workers must observe cancellation and return; the timeout does not terminate them.
