// Package logkit provides a configurable *zap.Logger for uber/fx applications.
//
// The module allows configuration for different environments (e.g., "production"
// for structured JSON logs, "development" for human-readable console logs)
// and log levels. It also automatically logs service metadata on startup and
// ensures log buffers are flushed on shutdown.
package logkit

import (
	"context"
	"fmt"
	"strings"

	"github.com/froppa/stackkit/kits/runtimeinfo"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Module provides a configured *zap.Logger and *zap.SugaredLogger to the Fx
// application container.
func Module() fx.Option {
	return fx.Options(
		// Provide a default config. Users can override this in their application
		// by providing their own logger.Config.
		fx.Provide(func() Config {
			return Config{
				// "production" is a safe default for structured, efficient logging.
				Encoding: "production",
				Level:    "info",
			}
		}),
		fx.Provide(New),
		fx.Provide(func(log *zap.Logger) *zap.SugaredLogger {
			return log.Sugar()
		}),
		fx.Invoke(RegisterHooks),
	)
}

// Config defines the configuration for the logger.
type Config struct {
	// Encoding sets the logger's output format. Use "production|json" for JSON
	// or "development" for a human-readable console format.
	Encoding string `yaml:"encoding" validate:"required,oneof=production prod json development dev console"`

	// Level is the minimum log level to record, e.g., "debug", "info", "warn".
	Level string `yaml:"level" validate:"required,oneof=debug info warn error dpanic panic fatal"`
}

// New constructs a new *zap.Logger based on the provided configuration.
// It enriches the logger with application metadata from the runtimeinfo package.
func New(cfg Config) (*zap.Logger, error) {
	var zapCfg zap.Config
	switch strings.ToLower(cfg.Encoding) {
	case "prod", "production", "json":
		zapCfg = zap.NewProductionConfig()
	case "dev", "development", "console":
		zapCfg = zap.NewDevelopmentConfig()
		// Use a more readable time format for development.
		zapCfg.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05")
	default:
		return nil, fmt.Errorf("unknown logger encoding: %q", cfg.Encoding)
	}

	// Parse and set the log level.
	level, err := zapcore.ParseLevel(cfg.Level)
	if err != nil {
		return nil, fmt.Errorf("invalid log level: %w", err)
	}
	zapCfg.Level = zap.NewAtomicLevelAt(level)

	// Build the logger.
	logger, err := zapCfg.Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build zap logger: %w", err)
	}

	// Add permanent fields from the runtimeinfo package.
	return logger.With(runtimeinfo.Fields()...), nil
}

// registerHooks attaches OnStart and OnStop hooks to the application lifecycle.
func RegisterHooks(lc fx.Lifecycle, log *zap.Logger) {
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			log.Info("Service starting",
				zap.String("service", runtimeinfo.Name),
				zap.String("version", runtimeinfo.Version),
				zap.String("commit", runtimeinfo.Commit),
			)
			return nil
		},
		OnStop: func(ctx context.Context) error {
			log.Info("Service stopping")
			// Sync flushes any buffered log entries. It's crucial for ensuring
			// logs are not lost on shutdown. We ignore the error, as it's
			// often a benign "inappropriate ioctl for device" on Linux.
			_ = log.Sync()
			return nil
		},
	})
}
