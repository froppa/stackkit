package httpkit_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	httpfx "github.com/froppa/stackkit/kits/httpkit"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

// --- NewListener ---

func TestNewListener_Binds(t *testing.T) {
	ln, err := httpfx.NewListener(&httpfx.Config{Addr: "127.0.0.1:0"})
	require.NoError(t, err)
	require.NotNil(t, ln)
	require.NoError(t, ln.Close())
}

func TestNewListener_InvalidAddr(t *testing.T) {
	_, err := httpfx.NewListener(&httpfx.Config{Addr: "??"})
	require.Error(t, err)
}

// --- NewMux ---

func TestNewMux_WithAndWithoutPprof(t *testing.T) {
	// no pprof, only custom handler
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	})
	mux := httpfx.NewMux(httpfx.Params{
		Cfg: &httpfx.Config{EnablePprof: false},
		Handlers: []httpfx.Handler{
			{Pattern: "/custom", Handler: h},
		},
	})

	// call /custom
	req := httptest.NewRequest("GET", "/custom", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
	require.Equal(t, "ok", rr.Body.String())

	// with pprof enabled
	mux2 := httpfx.NewMux(httpfx.Params{
		Cfg:      &httpfx.Config{EnablePprof: true},
		Handlers: nil,
	})
	req2 := httptest.NewRequest("GET", "/debug/pprof/", nil)
	rr2 := httptest.NewRecorder()
	mux2.ServeHTTP(rr2, req2)
	require.GreaterOrEqual(t, rr2.Code, 200)
	require.Less(t, rr2.Code, 500)
}

// --- Fx Module Lifecycle ---

func TestModule_StartStopWithHandler(t *testing.T) {
	var listenerPort int

	app := fx.New(
		// supply test config + logger
		fx.Replace(&httpfx.Config{Addr: "127.0.0.1:0"}),
		fx.Provide(func() *zap.Logger { return zaptest.NewLogger(t) }),

		// add a handler
		fx.Provide(fx.Annotate(
			func() httpfx.Handler {
				return httpfx.Handler{
					Pattern: "/ping",
					Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						_, _ = io.WriteString(w, "pong")
					}),
				}
			},
			fx.ResultTags(`group:"http.handlers"`),
		)),

		httpfx.Module(),

		// capture listener port
		fx.Invoke(func(l net.Listener) {
			listenerPort = l.Addr().(*net.TCPAddr).Port
		}),
	)

	// Cleanup
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = app.Stop(ctx)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, app.Start(ctx))

	// wait for server
	url := "http://127.0.0.1:" + strconv.Itoa(listenerPort) + "/ping"
	require.NoError(t, waitForOK(url, 20, 50*time.Millisecond))

	// explicit stop
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	require.NoError(t, app.Stop(stopCtx))
}

// --- Helper ---

func waitForOK(url string, tries int, delay time.Duration) error {
	client := &http.Client{Timeout: 200 * time.Millisecond}
	for i := 0; i < tries; i++ {
		resp, err := client.Get(url)
		if err == nil {
			ok := resp.StatusCode < 500
			_ = resp.Body.Close()
			if ok {
				return nil
			}
		}
		time.Sleep(delay)
	}
	return fmt.Errorf("server not ready: %s", url)
}
