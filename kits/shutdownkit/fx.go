// Package shutdownkit integrates the core logic from the `signals` package
// with the Uber Fx lifecycle. It provides a two-stage shutdown context
// and a shared WaitGroup for managing background goroutines.
package shutdownkit

import (
	"context"
	"sync"
	"time"

	"github.com/froppa/stackkit/kits/signals"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// Option configures Module.
type Option func(*opts)

type opts struct {
	timeout time.Duration
}

// WithTimeout overrides the graceful wait bound during shutdown.
// If not set or <=0, defaults to 10s. Keep in sync with fx.StopTimeout if used.
func WithTimeout(d time.Duration) Option {
	return func(o *opts) { o.timeout = d }
}

// ctxOut exports contexts only. We avoid re-providing Shutdown/WG to prevent duplicates.
type ctxOut struct {
	fx.Out
	Graceful context.Context `name:"graceful"`
	Force    context.Context `name:"force"`
}

// Module wires a single shutdown coordinator and integrates it with Fx lifecycle.
// Provides:
//   - context.Context `name:"graceful"`
//   - context.Context `name:"force"`
//   - *sync.WaitGroup
func Module(opt ...Option) fx.Option {
	cfg := opts{timeout: 10 * time.Second}
	for _, o := range opt {
		o(&cfg)
	}
	return fx.Options(
		// Single shared WaitGroup
		fx.Provide(func() *sync.WaitGroup { return &sync.WaitGroup{} }),

		// Single Shutdown coordinator (no OS signal handling here; Fx.Run owns signals)
		fx.Provide(signals.New),

		// Export named contexts only
		fx.Provide(func(s *signals.Shutdown) ctxOut {
			return ctxOut{
				Graceful: s.Graceful(),
				Force:    s.Force(),
			}
		}),

		// On stop: trigger graceful, then bounded wait; escalate to force after timeout
		fx.Invoke(func(lc fx.Lifecycle, log *zap.Logger, s *signals.Shutdown) {
			lc.Append(fx.Hook{
				OnStop: func(context.Context) error {
					log.Info("shutdown: initiating graceful")
					s.TriggerGraceful()
					s.Wait(cfg.timeout)
					log.Info("shutdown: completed")
					return nil
				},
			})
		}),
	)
}

// Go runs fn in a managed goroutine tied to the shared WaitGroup.
// Use this for background work that must complete or exit on shutdown.
func Go(wg *sync.WaitGroup, fn func()) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		fn()
	}()
}
