# Signals Module

Graceful and forced shutdown orchestration.

## Features

- Handles `SIGINT` and `SIGTERM` for CLI tools or services.
- Provides two contexts:
  - **Graceful**: cancelled on first signal.
  - **Force**: cancelled after timeout or second signal.
- WaitGroup support for in-flight goroutines.

## Usage

```go
wg := &sync.WaitGroup{}
s := signals.NewWithSignals(context.Background(), wg)

go func() {
    wg.Add(1)
    defer wg.Done()
    <-s.Graceful().Done() // cleanup
}()

s.Wait(10 * time.Second) // blocks until done or force timeout
```
