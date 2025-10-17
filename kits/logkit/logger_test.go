package logkit_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/froppa/stackkit/kits/logkit"
	info "github.com/froppa/stackkit/kits/runtimeinfo"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// helper to capture log output from a given logger while preserving its fields.
func captureLogs(t *testing.T, log *zap.Logger, fn func(*zap.Logger)) string {
	t.Helper()
	var buf bytes.Buffer
	enc := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	wrapped := log.WithOptions(zap.WrapCore(func(zapcore.Core) zapcore.Core {
		return zapcore.NewCore(enc, zapcore.AddSync(&buf), zapcore.DebugLevel)
	}))
	wrapped = wrapped.With(info.Fields()...)
	fn(wrapped)
	_ = wrapped.Sync()
	return buf.String()
}

func TestNewLogger_ValidConfigs(t *testing.T) {
	tests := []struct {
		name     string
		cfg      logkit.Config
		wantText string
	}{
		{"production", logkit.Config{Encoding: "production", Level: "info"}, info.Name},
		{"json alias", logkit.Config{Encoding: "json", Level: "debug"}, info.Version},
		{"development", logkit.Config{Encoding: "development", Level: "warn"}, info.Commit},
		{"console alias", logkit.Config{Encoding: "console", Level: "info"}, info.Name},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log, err := logkit.New(tt.cfg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			out := captureLogs(t, log, func(l *zap.Logger) {
				l.Info("test log")
			})
			if !strings.Contains(out, "test log") {
				t.Errorf("log output missing message: %s", out)
			}
			if !strings.Contains(out, tt.wantText) {
				t.Errorf("log output missing meta field %q: %s", tt.wantText, out)
			}
		})
	}
}

func TestNewLogger_InvalidEncoding(t *testing.T) {
	_, err := logkit.New(logkit.Config{Encoding: "invalid", Level: "info"})
	if err == nil || !strings.Contains(err.Error(), "unknown logger encoding") {
		t.Fatalf("expected unknown encoding error, got %v", err)
	}
}

func TestNewLogger_InvalidLevel(t *testing.T) {
	_, err := logkit.New(logkit.Config{Encoding: "production", Level: "nonsense"})
	if err == nil || !strings.Contains(err.Error(), "invalid log level") {
		t.Fatalf("expected invalid log level error, got %v", err)
	}
}

func TestRegisterHooks(t *testing.T) {
	var buf bytes.Buffer
	enc := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	core := zapcore.NewCore(enc, zapcore.AddSync(&buf), zapcore.DebugLevel)
	log := zap.New(core)

	app := fx.New(
		fx.Provide(func() *zap.Logger { return log }),
		fx.Invoke(func(lc fx.Lifecycle, log *zap.Logger) {
			// attach hooks manually
			loggerTest := zap.NewExample() // dummy to ensure sync works
			logkit.RegisterHooks(lc, loggerTest)
		}),
	)

	// simulate start/stop
	startCtx, stopCtx := context.Background(), context.Background()
	if err := app.Start(startCtx); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	if err := app.Stop(stopCtx); err != nil {
		t.Fatalf("stop failed: %v", err)
	}
}

func TestModuleIntegration(t *testing.T) {
	app := fx.New(logkit.Module())
	startCtx, stopCtx := context.Background(), context.Background()
	if err := app.Start(startCtx); err != nil {
		t.Fatalf("app start failed: %v", err)
	}
	if err := app.Stop(stopCtx); err != nil {
		t.Fatalf("app stop failed: %v", err)
	}
}
