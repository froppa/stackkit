// Package healthkit provides liveness and readiness reporting for uber/fx
// applications. It is designed for use with container orchestrators and load
// balancers like Kubernetes.
//
// Two integration modes are supported:
//
//  1. Dedicated server (Module): starts its own HTTP server on a configurable
//     port. This is the recommended approach as it isolates health checks
//     from application traffic.
//  2. Mux attachment (MuxModule): attaches a /health handler to an existing
//     *http.ServeMux provided by the application. Useful if you already run
//     an HTTP server and want to avoid a second port.
package healthkit

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/froppa/stackkit/kits/configkit"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// ServerModule provides a self-contained health server on a dedicated port.
// It includes the core Health service and invokes a dedicated HTTP server.
func ServerModule() fx.Option {
	return fx.Module("health/server",
		fx.Provide(configkit.ProvideFromKey[Config]("health")),
		fx.Provide(New),
		fx.Invoke(RegisterServer),
	)
}

// MuxModule provides health reporting attached to an existing *http.ServeMux.
// It includes the core Health service and invokes a handler registration.
func MuxModule() fx.Option {
	return fx.Module("health/mux",
		// CHANGE: Also provide the config here for consistency.
		fx.Provide(configkit.ProvideFromKey[Config]("health")),
		fx.Provide(New),
		fx.Invoke(RegisterMux),
	)
}

// Config defines configuration for the Health service.
type Config struct {
	// Port is the network address for the dedicated health server.
	// Defaults to ":8081" if not set.
	// Only used by ServerModule(), ignored by MuxModule().
	Port string `yaml:"port"`

	// StartupDelay is the duration to wait after the application has started
	// before reporting readiness. Defaults to 200ms if not set.
	StartupDelay time.Duration `yaml:"startup_delay"`
}

// Health tracks and reports liveness and readiness state.
type Health struct {
	ready atomic.Bool
	live  atomic.Bool
	cfg   *Config
	log   *zap.Logger
}

// Params defines the dependencies required to construct the Health service.
type Params struct {
	fx.In

	LC     fx.Lifecycle
	Logger *zap.Logger
	// The Config is now marked as optional, as it may not be present in the YAML.
	Config *Config `optional:"true"`
}

// New constructs a new Health service and attaches hooks to manage its state
// according to the application's lifecycle.
func New(p Params) *Health {
	cfg := &Config{
		Port:         ":8081",
		StartupDelay: 200 * time.Millisecond,
	}
	if p.Config != nil {
		cfg = &Config{
			Port:         p.Config.Port,
			StartupDelay: p.Config.StartupDelay,
		}
		if cfg.Port == "" {
			cfg.Port = ":8081"
		}
		if cfg.StartupDelay == 0 {
			cfg.StartupDelay = 200 * time.Millisecond
		}
	}

	h := &Health{
		cfg: cfg,
		log: p.Logger.With(zap.String("component", "health")),
	}

	// This lifecycle hook is independent of the server and manages the
	// readiness/liveness state for both Module and MuxModule.
	p.LC.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			h.live.Store(true)
			h.ready.Store(false)
			go func() {
				time.Sleep(h.cfg.StartupDelay)
				h.ready.Store(true)
				h.log.Info("service is ready")
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			h.ready.Store(false)
			h.live.Store(false)
			h.log.Info("service is stopping")
			return nil
		},
	})

	return h
}

// response is the JSON structure returned by the health endpoint.
type response struct {
	Status string `json:"status"`
	Ready  bool   `json:"ready"`
	Live   bool   `json:"live"`
}

// handler returns an http.Handler that serves the health status.
func (h *Health) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}

		resp := response{
			Status: "ok",
			Live:   h.live.Load(),
			Ready:  h.ready.Load(),
		}
		code := http.StatusOK

		if !resp.Live {
			resp.Status = "unhealthy"
			code = http.StatusServiceUnavailable
		} else if !resp.Ready {
			resp.Status = "initializing"
			code = http.StatusServiceUnavailable
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(code)

		if err := json.NewEncoder(w).Encode(resp); err != nil {
			h.log.Error("failed to write health response", zap.Error(err))
		}
	})
}

// RegisterServer creates a dedicated HTTP server and registers it with the
// application lifecycle. This is used by ServerModule().
func RegisterServer(lc fx.Lifecycle, h *Health) {
	mux := http.NewServeMux()
	mux.Handle("/health", h.handler())
	server := &http.Server{
		Addr:    h.cfg.Port,
		Handler: mux,
	}

	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go func() {
				h.log.Info("starting health server", zap.String("addr", server.Addr))
				if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					h.log.Error("health server failed", zap.Error(err))
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			h.log.Info("stopping health server")
			return server.Shutdown(ctx)
		},
	})
}

// RegisterMux attaches the health handler to a Mux provided in the Fx container.
// This is used by MuxModule().
func RegisterMux(mux *http.ServeMux, h *Health) {
	mux.Handle("/health", h.handler())
}
