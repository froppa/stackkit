// Package httpkit provides an HTTP server with Uber Fx integration.
// It includes configuration loading, optional pprof endpoints,
// structured logging, graceful shutdown, and a simple mechanism
// for services to register their own handlers.
package httpkit

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/froppa/stackkit/kits/configkit"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

func init() { configkit.RegisterKnown("http", (*Config)(nil)) }

// Config holds HTTP server configuration.
type Config struct {
	// Addr is the listen address, e.g. ":8080".
	Addr string `yaml:"addr" validate:"required"`

	// ReadTimeoutMS sets the maximum duration for reading the request in ms.
	ReadTimeoutMS int `yaml:"read_timeout_ms" validate:"gte=0"`

	// WriteTimeoutMS sets the maximum duration for writing the response in ms.
	WriteTimeoutMS int `yaml:"write_timeout_ms" validate:"gte=0"`

	// EnablePprof enables /debug/pprof endpoints if true. Default false.
	EnablePprof bool `yaml:"enable_pprof"`
}

// Handler allows services to register additional HTTP routes via Fx groups.
type Handler struct {
	Pattern string
	Handler http.Handler
}

// Params is used by NewMux to pull in grouped handlers.
type Params struct {
	fx.In
	Cfg      *Config
	Handlers []Handler `group:"http.handlers"`
}

// Module provides HTTP server configuration and lifecycle management for Fx.
//
// It wires:
//   - Config from "http" subtree
//   - net.Listener bound to Addr
//   - *http.ServeMux with optional pprof + group handlers
//   - Server lifecycle with graceful shutdown
//
// To register routes from a service:
//
//	fx.Provide(func() http.Handler {
//	    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//	        w.Write([]byte("hello"))
//	    })
//	})
//
// Or directly provide a route:
//
//	fx.Provide(func() http.Handler {
//	    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//	        w.Write([]byte("pong"))
//	    })
//	})
func Module() fx.Option {
	return fx.Options(
		fx.Provide(configkit.ProvideFromKey[Config]("http")),
		fx.Provide(NewListener),
		fx.Provide(NewMux),
		fx.Invoke(registerHTTPServer),
	)
}

// NewListener binds a TCP listener to the configured Addr.
func NewListener(cfg *Config) (net.Listener, error) {
	return net.Listen("tcp", cfg.Addr)
}

// NewMux builds a ServeMux with optional pprof and all grouped handlers.
func NewMux(p Params) *http.ServeMux {
	mux := http.NewServeMux()

	if p.Cfg.EnablePprof {
		mux.Handle("/debug/pprof/", otelhttp.NewHandler(http.HandlerFunc(pprof.Index), "pprof.index"))
		mux.Handle("/debug/pprof/cmdline", otelhttp.NewHandler(http.HandlerFunc(pprof.Cmdline), "pprof.cmdline"))
		mux.Handle("/debug/pprof/profile", otelhttp.NewHandler(http.HandlerFunc(pprof.Profile), "pprof.profile"))
		mux.Handle("/debug/pprof/symbol", otelhttp.NewHandler(http.HandlerFunc(pprof.Symbol), "pprof.symbol"))
		mux.Handle("/debug/pprof/trace", otelhttp.NewHandler(http.HandlerFunc(pprof.Trace), "pprof.trace"))
	}

	for _, r := range p.Handlers {
		mux.Handle(r.Pattern, r.Handler)
	}

	return mux
}

// registerHTTPServer wires the HTTP server into the Fx lifecycle.
func registerHTTPServer(
	lc fx.Lifecycle,
	listener net.Listener,
	cfg *Config,
	mux *http.ServeMux,
	log *zap.Logger,
) {
	srv := &http.Server{
		Addr:    listener.Addr().String(),
		Handler: mux,
	}
	if cfg.ReadTimeoutMS > 0 {
		srv.ReadTimeout = time.Duration(cfg.ReadTimeoutMS) * time.Millisecond
	}
	if cfg.WriteTimeoutMS > 0 {
		srv.WriteTimeout = time.Duration(cfg.WriteTimeoutMS) * time.Millisecond
	}

	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go func() {
				log.Info("http.start", zap.String("addr", cfg.Addr))
				if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
					log.Error("http.serve_error", zap.Error(err))
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			log.Info("http.stop")
			if err := srv.Shutdown(ctx); err != nil {
				if errors.Is(err, context.DeadlineExceeded) {
					log.Warn("http.shutdown_timeout")
					return srv.Close()
				}
				return err
			}
			log.Info("http.stopped_clean")
			return nil
		},
	})
}
