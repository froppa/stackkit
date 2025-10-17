// Package telemetry provides an opinionated OpenTelemetry (OTEL) module for uber/fx.
//
// It simplifies integration by automatically configuring and wiring the necessary OTEL
// components into the Fx dependency injection container.
package telemetry

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/froppa/stackkit/kits/configkit"
	"github.com/froppa/stackkit/kits/runtimeinfo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

func init() { configkit.RegisterKnown("telemetry", (*Config)(nil)) }

// Module provides all OpenTelemetry components and lifecycle hooks for an Fx application.
// It is the main entry point for using this package.
func Module() fx.Option {
	return fx.Options(
		fx.Provide(configkit.ProvideFromKey[Config]("telemetry")),
		fx.Provide(NewProviders),
		fx.Invoke(registerShutdown),
		fx.Invoke(installGlobals),
	)
}

type globalDeps struct {
	fx.In
	TracerProvider *sdktrace.TracerProvider
	MeterProvider  *sdkmetric.MeterProvider
}

func installGlobals(d globalDeps) {
	if d.TracerProvider != nil {
		otel.SetTracerProvider(d.TracerProvider)
	}
	if d.MeterProvider != nil {
		otel.SetMeterProvider(d.MeterProvider)
	}
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))
}

// Config defines the settings for the OpenTelemetry module, loaded from a YAML file.
type Config struct {
	// ServiceName identifies the service in telemetry data.
	// Overridden by the OTEL_SERVICE_NAME environment variable.
	ServiceName string `yaml:"service_name" validate:"omitempty"`

	// ServiceVersion is the version of the service.
	ServiceVersion string `yaml:"service_version" validate:"omitempty"`

	// Environment is the deployment environment (e.g., "production", "staging").
	Environment string `yaml:"environment" validate:"omitempty"`

	// OTLPEndpoint is the host:port address of the OTLP collector.
	// If set, OTLP/gRPC exporters for traces and metrics are enabled.
	// Overridden by the OTEL_EXPORTER_OTLP_ENDPOINT environment variable.
	OTLPEndpoint string `yaml:"otlp_endpoint" validate:"omitempty"`

	// Insecure disables TLS when connecting to the OTLP endpoint.
	Insecure bool `yaml:"insecure"`

	// Disabled completely disables the OpenTelemetry SDK. If true, all other
	// tracing and metrics settings are ignored, and no-op providers are configured.
	// Overridden by the OTEL_SDK_DISABLED environment variable.
	Disabled *bool `yaml:"disabled"`

	// TracingEnabled explicitly enables or disables tracing.
	// If this is not set, tracing is automatically enabled if OTLPEndpoint is present.
	// This is ignored if 'Disabled' is true.
	TracingEnabled *bool `yaml:"tracing_enabled"`

	// MetricsEnabled explicitly enables or disables metrics.
	// If this is not set, metrics are automatically enabled if OTLPEndpoint is present.
	// This is ignored if 'Disabled' is true.
	MetricsEnabled *bool `yaml:"metrics_enabled"`

	// TraceSampler defines the sampling strategy.
	// Valid options are "parent_ratio" (default), "always_on", "always_off".
	TraceSampler string `yaml:"trace_sampler" validate:"omitempty,oneof=parent_ratio always_on always_off"`

	// TraceSampleRate is the sampling rate for the "parent_ratio" sampler (e.g., 0.5 for 50%).
	TraceSampleRate float64 `yaml:"trace_sample_rate" validate:"gte=0,lte=1"`

	// ExportInterval is the frequency at which metrics are exported.
	ExportInterval time.Duration `yaml:"export_interval" validate:"gte=0"`

	// ResourceAttributes are additional key-value pairs to add to the resource identity.
	ResourceAttributes map[string]string `yaml:"resource_attributes" validate:"omitempty,dive,keys,required,endkeys,required"`
}

// Result is an fx.Out struct that provides all OTEL components to the Fx container.
// This allows other services to depend on specific components (e.g., trace.Tracer)
// instead of a monolithic struct.
type Result struct {
	fx.Out
	TracerProvider *sdktrace.TracerProvider
	MeterProvider  *sdkmetric.MeterProvider
	Tracer         trace.Tracer
	Meter          metric.Meter
}

// NewProviders is an Fx constructor that builds the OTEL providers based on the loaded Config.
// It is responsible for setting up the resource, exporters, and the tracer/meter providers.
func NewProviders(ctx context.Context, cfg *Config, log *zap.Logger) (Result, error) {
	out := Result{}
	if cfg == nil {
		return out, errors.New("telemetry config is nil")
	}

	applyConfigDefaults(cfg)

	res, err := buildResource(*cfg)
	if err != nil {
		return out, fmt.Errorf("failed to build telemetry resource: %w", err)
	}

	if *cfg.Disabled {
		tp := sdktrace.NewTracerProvider(
			sdktrace.WithSampler(sdktrace.NeverSample()),
			sdktrace.WithResource(res),
		)
		mp := sdkmetric.NewMeterProvider(sdkmetric.WithResource(res))
		out.TracerProvider, out.MeterProvider = tp, mp
		out.Tracer, out.Meter = tp.Tracer(cfg.ServiceName), mp.Meter(cfg.ServiceName)
		log.Info("telemetry disabled")
		return out, nil
	}

	tp, err := buildTracerProvider(ctx, *cfg, res)
	if err != nil {
		return out, err
	}
	out.TracerProvider = tp
	out.Tracer = tp.Tracer(cfg.ServiceName)

	mp, err := buildMeterProvider(ctx, *cfg, res)
	if err != nil {
		return out, err
	}
	out.MeterProvider = mp
	out.Meter = mp.Meter(cfg.ServiceName)

	if *cfg.TracingEnabled && cfg.OTLPEndpoint == "" {
		log.Warn("tracing enabled but no OTLP endpoint set")
	}
	if *cfg.MetricsEnabled && cfg.OTLPEndpoint == "" {
		log.Warn("metrics enabled but no OTLP endpoint set")
	}

	log.Info("telemetry initialized",
		zap.String("service.name", cfg.ServiceName),
		zap.String("service.version", cfg.ServiceVersion),
		zap.String("deployment.environment", cfg.Environment),
		zap.Bool("sdk.disabled", *cfg.Disabled),
		zap.Bool("tracing.enabled", *cfg.TracingEnabled),
		zap.Bool("metrics.enabled", *cfg.MetricsEnabled),
		zap.String("otlp.endpoint", cfg.OTLPEndpoint),
	)
	return out, nil
}

// applyConfigDefaults populates the Config struct with values from environment
// variables, the meta package, and hardcoded defaults.
func applyConfigDefaults(cfg *Config) {
	// Highest precedence: standard env vars
	if envEndpoint := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")); envEndpoint != "" {
		cfg.OTLPEndpoint = envEndpoint
	}
	if envServiceName := strings.TrimSpace(os.Getenv("OTEL_SERVICE_NAME")); envServiceName != "" {
		cfg.ServiceName = envServiceName
	}
	if envSDKDisabled := strings.TrimSpace(os.Getenv("OTEL_SDK_DISABLED")); envSDKDisabled != "" {
		if val, err := strconv.ParseBool(envSDKDisabled); err == nil {
			cfg.Disabled = &val
		}
	}

	// Next precedence: runtimeinfo package
	if cfg.ServiceName == "" {
		cfg.ServiceName = runtimeinfo.Name
	}
	if cfg.ServiceVersion == "" {
		cfg.ServiceVersion = runtimeinfo.Version
	}
	if cfg.Environment == "" {
		cfg.Environment = coalesceEnv("ENV", "APP_ENV", "GO_ENV")
		if cfg.Environment == "" {
			cfg.Environment = "dev"
		}
	}

	// Lowest precedence: hardcoded defaults
	if cfg.TraceSampleRate <= 0 {
		cfg.TraceSampleRate = 1.0
	}
	if cfg.ExportInterval <= 0 {
		cfg.ExportInterval = 30 * time.Second
	}

	// Set defaults for boolean pointers if they are nil
	setDefaultBool(&cfg.Disabled, false)
	enabledByEndpoint := cfg.OTLPEndpoint != "" && !*cfg.Disabled
	setDefaultBool(&cfg.TracingEnabled, enabledByEndpoint)
	setDefaultBool(&cfg.MetricsEnabled, enabledByEndpoint)

	// Final check: if the entire SDK is disabled, tracing and metrics must also be disabled.
	if *cfg.Disabled {
		disabledState := false
		cfg.TracingEnabled = &disabledState
		cfg.MetricsEnabled = &disabledState
	}
}

// buildResource creates the OTEL resource by merging attributes from the default
// resource, configuration, and runtime metadata package.
func buildResource(cfg Config) (*sdkresource.Resource, error) {
	// Standard attributes
	attrs := []attribute.KeyValue{
		semconv.ServiceName(cfg.ServiceName),
		semconv.ServiceVersion(cfg.ServiceVersion),
		semconv.DeploymentEnvironmentName(cfg.Environment),
	}
	// Add the standard disabled attribute if the SDK is disabled.
	if *cfg.Disabled {
		attrs = append(attrs, attribute.Bool("otel.sdk.disabled", true))
	}
	mainAttrs := sdkresource.NewWithAttributes(semconv.SchemaURL, attrs...)

	// Extra attributes from runtime metadata package
	metaAttrs := sdkresource.NewWithAttributes(
		semconv.SchemaURL,
		runtimeinfo.OTELAttributes()...,
	)

	// Extra attributes from config file
	var extraConfigAttrs []attribute.KeyValue
	for k, v := range cfg.ResourceAttributes {
		extraConfigAttrs = append(extraConfigAttrs, attribute.String(k, v))
	}
	extraAttrs := sdkresource.NewWithAttributes(semconv.SchemaURL, extraConfigAttrs...)

	// Merge all resource sources.
	res, err := sdkresource.Merge(sdkresource.Default(), mainAttrs)
	if err != nil {
		return nil, err
	}
	res, err = sdkresource.Merge(res, metaAttrs)
	if err != nil {
		return nil, err
	}
	return sdkresource.Merge(res, extraAttrs)
}

type shutdownDeps struct {
	fx.In

	TracerProvider *sdktrace.TracerProvider
	MeterProvider  *sdkmetric.MeterProvider
	Logger         *zap.Logger
	LC             fx.Lifecycle
}

// registerShutdown attaches a hook to the Fx application lifecycle to gracefully
// shut down the tracer and meter providers, ensuring all telemetry is flushed.
func registerShutdown(params shutdownDeps) {
	params.LC.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			params.Logger.Info("shutting down telemetry providers")
			// Create a new context for shutdown to avoid premature cancellation from Fx's OnStop context.
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			// Attempt both shutdowns and join errors to ensure both are attempted.
			return errors.Join(
				shutdownMeter(shutdownCtx, params.MeterProvider, params.Logger),
				shutdownTracer(shutdownCtx, params.TracerProvider, params.Logger),
			)
		},
	})
}

// buildTracerProvider creates a new trace provider with a configured sampler and exporter.
func buildTracerProvider(ctx context.Context, cfg Config, res *sdkresource.Resource) (*sdktrace.TracerProvider, error) {
	var sampler sdktrace.Sampler
	switch cfg.TraceSampler {
	case "always_on":
		sampler = sdktrace.AlwaysSample()
	case "always_off":
		sampler = sdktrace.NeverSample()
	case "parent_ratio", "":
		sampler = sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.TraceSampleRate))
	default:
		return nil, fmt.Errorf("unknown trace sampler: %q", cfg.TraceSampler)
	}

	if *cfg.TracingEnabled && cfg.OTLPEndpoint != "" {
		opts := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(cfg.OTLPEndpoint)}
		if cfg.Insecure {
			opts = append(opts, otlptracegrpc.WithInsecure())
		}
		exp, err := otlptracegrpc.New(ctx, opts...)
		if err != nil {
			return nil, fmt.Errorf("otlp trace exporter: %w", err)
		}
		return sdktrace.NewTracerProvider(
			sdktrace.WithBatcher(exp),
			sdktrace.WithResource(res),
			sdktrace.WithSampler(sampler),
		), nil
	}

	// Return a provider with no exporter if tracing is disabled or no endpoint is set.
	return sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	), nil
}

// buildMeterProvider creates a new meter provider with a configured exporter.
func buildMeterProvider(ctx context.Context, cfg Config, res *sdkresource.Resource) (*sdkmetric.MeterProvider, error) {
	if *cfg.MetricsEnabled && cfg.OTLPEndpoint != "" {
		opts := []otlpmetricgrpc.Option{otlpmetricgrpc.WithEndpoint(cfg.OTLPEndpoint)}
		if cfg.Insecure {
			opts = append(opts, otlpmetricgrpc.WithInsecure())
		}
		exp, err := otlpmetricgrpc.New(ctx, opts...)
		if err != nil {
			return nil, fmt.Errorf("otlp metric exporter: %w", err)
		}
		reader := sdkmetric.NewPeriodicReader(exp, sdkmetric.WithInterval(cfg.ExportInterval))
		return sdkmetric.NewMeterProvider(
			sdkmetric.WithReader(reader),
			sdkmetric.WithResource(res),
		), nil
	}

	// Return a provider with no exporter if metrics are disabled or no endpoint is set.
	return sdkmetric.NewMeterProvider(sdkmetric.WithResource(res)), nil
}

// shutdownTracer gracefully stops the tracer provider.
func shutdownTracer(ctx context.Context, tp *sdktrace.TracerProvider, log *zap.Logger) error {
	if tp == nil {
		return nil
	}
	if err := tp.Shutdown(ctx); err != nil {
		log.Error("failed to shut down telemetry tracer provider", zap.Error(err))
		return err
	}
	return nil
}

// shutdownMeter gracefully stops the meter provider.
func shutdownMeter(ctx context.Context, mp *sdkmetric.MeterProvider, log *zap.Logger) error {
	if mp == nil {
		return nil
	}
	if err := mp.Shutdown(ctx); err != nil {
		log.Error("failed to shut down telemetry meter provider", zap.Error(err))
		return err
	}
	return nil
}

// coalesceEnv returns the value of the first non-empty environment variable.
func coalesceEnv(keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}

// setDefaultBool sets a bool pointer to the default value if it is nil.
func setDefaultBool(b **bool, defaultValue bool) {
	if *b == nil {
		*b = &defaultValue
	}
}
