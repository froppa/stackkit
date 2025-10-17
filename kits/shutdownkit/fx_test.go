package shutdownkit_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/froppa/stackkit/kits/shutdownkit"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

type ShutdownDeps struct {
	fx.In

	Graceful context.Context `name:"graceful"`
	Force    context.Context `name:"force"`
	WG       *sync.WaitGroup
}

func TestNew(t *testing.T) {
	var got ShutdownDeps

	app := fx.New(
		shutdownkit.Module(),
		fx.Provide(func() *zap.Logger { return zaptest.NewLogger(t) }),
		fx.Invoke(func(sd ShutdownDeps) { got = sd }),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, app.Start(ctx))

	ctxStop, cancelStop := context.WithTimeout(context.Background(), time.Second)
	defer cancelStop()
	require.NoError(t, app.Stop(ctxStop))

	require.NotNil(t, got.Graceful)
	require.NotNil(t, got.Force)
	require.NotNil(t, got.WG)
}

func TestModule_ProvidesShutdownContexts(t *testing.T) {
	app := fx.New(
		shutdownkit.Module(),
		fx.Provide(func() *zap.Logger { return zaptest.NewLogger(t) }),
		fx.Invoke(func(sd ShutdownDeps) {
			require.NotNil(t, sd.Graceful)
			require.NotNil(t, sd.Force)
			require.NotNil(t, sd.WG)
		}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, app.Start(ctx))

	ctxStop, cancelStop := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelStop()
	require.NoError(t, app.Stop(ctxStop))
}

func TestShutdownLifecycle(t *testing.T) {
	var sd ShutdownDeps
	gracefulObserved := make(chan struct{}, 1)

	app := fx.New(
		shutdownkit.Module(),
		fx.Provide(func() *zap.Logger { return zaptest.NewLogger(t) }),
		fx.Invoke(func(d ShutdownDeps) {
			sd = d
			// observe graceful cancellation outside OnStop to avoid ordering issues
			go func() {
				<-sd.Graceful.Done()
				gracefulObserved <- struct{}{}
			}()
			// run a managed goroutine
			sd.WG.Add(1)
			go func() {
				defer sd.WG.Done()
				<-sd.Graceful.Done()
			}()
		}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, app.Start(ctx))

	ctxStop, cancelStop := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelStop()
	require.NoError(t, app.Stop(ctxStop))

	select {
	case <-gracefulObserved:
		// ok
	default:
		t.Fatal("expected graceful context to be cancelled during Stop")
	}
}
