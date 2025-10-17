// Package signals provides a framework-agnostic coordinator for graceful
// and forced shutdowns.
// For standalone applications, use NewWithSignals() to handle OS signals.
// For integration with frameworks like Uber Fx, use New() and trigger
// shutdown manually. See the accompanying `shutdownkit` package for an example.
package signals

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// Shutdown coordinates application shutdown.
//
// It provides:
//   - Graceful context: canceled on SIGINT/SIGTERM or manual trigger
//   - Force context: canceled if shutdown exceeds a timeout
//   - A WaitGroup for tracking in-flight goroutines
type Shutdown struct {
	gracefulCtx context.Context
	gracefulFn  context.CancelFunc

	forceCtx context.Context
	forceFn  context.CancelFunc

	wg *sync.WaitGroup
}

// New returns a Shutdown that does not listen for OS signals.
// Intended for Fx apps where lifecycle hooks initiate shutdown.
func New(wg *sync.WaitGroup) *Shutdown {
	return newShutdown(context.Background(), wg, false)
}

// NewWithSignals returns a Shutdown that listens for SIGINT/SIGTERM.
// Intended for stand-alone CLIs or background workers.
func NewWithSignals(ctx context.Context, wg *sync.WaitGroup) *Shutdown {
	return newShutdown(ctx, wg, true)
}

// newShutdown constructs a Shutdown, optionally listening for OS signals.
func newShutdown(ctx context.Context, wg *sync.WaitGroup, listen bool) *Shutdown {
	forceCtx, forceFn := context.WithCancel(ctx)
	gracefulCtx, gracefulFn := context.WithCancel(ctx)

	s := &Shutdown{
		gracefulCtx: gracefulCtx,
		gracefulFn:  gracefulFn,
		forceCtx:    forceCtx,
		forceFn:     forceFn,
		wg:          wg,
	}

	if listen {
		go func() {
			ch := make(chan os.Signal, 1)
			signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
			defer signal.Stop(ch)

			// Loop to handle the first signal and ignore subsequent ones.
			// The goroutine exits once the graceful context is canceled,
			// which can be triggered by a signal or programmatically.
			for {
				select {
				case <-ch:
					s.gracefulFn()
				case <-s.gracefulCtx.Done():
					return
				}
			}
		}()
	}

	return s
}

// Graceful returns the context canceled on SIGINT/SIGTERM or manual trigger.
func (s *Shutdown) Graceful() context.Context {
	return s.gracefulCtx
}

// Force returns the context canceled if shutdown exceeds timeout in Wait.
func (s *Shutdown) Force() context.Context {
	return s.forceCtx
}

// WaitGroup returns the tracked WaitGroup for in-flight goroutines.
func (s *Shutdown) WaitGroup() *sync.WaitGroup {
	return s.wg
}

// TriggerGraceful cancels the graceful context programmatically.
func (s *Shutdown) TriggerGraceful() {
	s.gracefulFn()
}

// Wait blocks until the WaitGroup drains or timeout elapses.
// If timeout triggers, the force context is canceled and Wait continues
// until all goroutines complete.
func (s *Shutdown) Wait(timeout time.Duration) {
	<-s.gracefulCtx.Done()

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.wg.Wait()
	}()

	select {
	case <-done:
		return
	case <-time.After(timeout):
		s.forceFn()
		<-done
	}
}
