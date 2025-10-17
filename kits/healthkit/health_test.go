package healthkit_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/froppa/stackkit/kits/configkit"
	"github.com/froppa/stackkit/kits/healthkit"
	"github.com/stretchr/testify/require"
	uber "go.uber.org/config"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
	"go.uber.org/zap"
)

// healthResponse matches the JSON structure returned by the health endpoint.
type healthResponse struct {
	Status string `json:"status"`
	Ready  bool   `json:"ready"`
	Live   bool   `json:"live"`
}

// checkHealthEndpoint is a helper function to query a health endpoint and assert its state.
func checkHealthEndpoint(t *testing.T, url string, wantStatus string, wantCode int, wantLive, wantReady bool) {
	t.Helper()

	res, err := http.Get(url)
	require.NoError(t, err, "HTTP GET request should not fail")
	defer func() {
		require.NoError(t, res.Body.Close())
	}()

	require.Equal(t, wantCode, res.StatusCode, "HTTP status code should match expected")
	require.Contains(t, res.Header.Get("Content-Type"), "application/json", "Content-Type should be application/json")

	var body healthResponse
	err = json.NewDecoder(res.Body).Decode(&body)
	require.NoError(t, err, "JSON decoding should not fail")

	require.Equal(t, wantStatus, body.Status, "Status field should match")
	require.Equal(t, wantLive, body.Live, "Live field should match")
	require.Equal(t, wantReady, body.Ready, "Ready field should match")
}

// getFreePort finds a free TCP port to use for the test server, avoiding conflicts.
func getFreePort(t *testing.T) string {
	t.Helper()
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	require.NoError(t, err)
	l, err := net.ListenTCP("tcp", addr)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, l.Close())
	}()
	return fmt.Sprintf(":%d", l.Addr().(*net.TCPAddr).Port)
}

// TestHealthModule contains sub-tests for the different integration modes.
func TestHealthModule(t *testing.T) {
	// A short startup delay for testing the "initializing" state.
	const testStartupDelay = 50 * time.Millisecond

	t.Run("ServerModule follows lifecycle", func(t *testing.T) {
		t.Parallel()

		var healthServerURL string
		testPort := getFreePort(t)

		yamlSrc := fmt.Sprintf("health:\n  port: \"%s\"\n  startup_delay: %s\n", testPort, testStartupDelay.String())

		app := fxtest.New(t,
			fx.Provide(zap.NewNop),
			configkit.Module(configkit.WithSources(uber.Source(bytes.NewBufferString(yamlSrc)))),
			healthkit.ServerModule(),
		)

		// The test needs the URL, so we construct it from the config.
		healthServerURL = "http://localhost" + testPort + "/health"

		// Start the Fx app. This triggers OnStart hooks.
		startCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, app.Start(startCtx), "Fx app should start without error")

		// 1. Check immediately after start: Should be live but not ready.
		checkHealthEndpoint(t, healthServerURL, "initializing", http.StatusServiceUnavailable, true, false)

		// 2. Wait for startup delay to pass.
		time.Sleep(testStartupDelay + 10*time.Millisecond)

		// 3. Check again: Should now be live AND ready.
		checkHealthEndpoint(t, healthServerURL, "ok", http.StatusOK, true, true)

		// Stop the Fx app. This triggers OnStop hooks.
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, app.Stop(stopCtx), "Fx app should stop without error")

		// 4. Check after stop: Should be not live and not ready.
		// We expect a connection error here since the server is shut down, but we test the state logic in the Mux test.
		res, err := http.Get(healthServerURL)
		if err == nil {
			require.NoError(t, res.Body.Close())
		}
		require.Error(t, err, "Expected a connection error after server shutdown")
	})

	t.Run("MuxModule follows lifecycle", func(t *testing.T) {
		t.Parallel()
		var h *healthkit.Health

		// Create a mux that our MuxModule will attach to.
		mux := http.NewServeMux()
		testServer := httptest.NewServer(mux)
		defer testServer.Close()
		healthServerURL := testServer.URL + "/health"

		yamlSrc := fmt.Sprintf("health:\n  startup_delay: %s\n", testStartupDelay.String())

		app := fxtest.New(t,
			fx.Provide(zap.NewNop),
			fx.Provide(func() *http.ServeMux { return mux }),
			configkit.Module(configkit.WithSources(uber.Source(bytes.NewBufferString(yamlSrc)))),
			healthkit.MuxModule(),
			fx.Populate(&h),
		)

		startCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, app.Start(startCtx), "Fx app should start without error")

		// 1. Check immediately after start: Should be live but not ready.
		checkHealthEndpoint(t, healthServerURL, "initializing", http.StatusServiceUnavailable, true, false)

		// 2. Wait for startup delay to pass.
		time.Sleep(testStartupDelay + 10*time.Millisecond)

		// 3. Check again: Should now be live AND ready.
		checkHealthEndpoint(t, healthServerURL, "ok", http.StatusOK, true, true)

		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, app.Stop(stopCtx), "Fx app should stop without error")

		// 4. Check after stop: Should be not live and not ready.
		// Because we control the test server, we can still make a request.
		checkHealthEndpoint(t, healthServerURL, "unhealthy", http.StatusServiceUnavailable, false, false)
	})

	t.Run("ServerModule works with default config", func(t *testing.T) {
		t.Parallel()

		// This test ensures the module works when no config is provided in the container.
		app := fxtest.New(t,
			fx.Provide(zap.NewNop),
			// No health.Config provided, forcing the module to use its internal defaults.
			healthkit.ServerModule(),
		)

		startCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, app.Start(startCtx), "Fx app should start without error with default config")

		// Check the default port to ensure the server started.
		// We expect it to be initializing.
		checkHealthEndpoint(t, "http://localhost:8081/health", "initializing", http.StatusServiceUnavailable, true, false)

		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, app.Stop(stopCtx), "Fx app should stop without error with default config")
	})
}
