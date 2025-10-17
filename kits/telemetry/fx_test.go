package telemetry

import (
	"context"
	"testing"
	"time"

	info "github.com/froppa/stackkit/kits/runtimeinfo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	fxtest "go.uber.org/fx/fxtest"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestInstallGlobals(t *testing.T) {
	prevTracer := otel.GetTracerProvider()
	prevMeter := otel.GetMeterProvider()
	prevProp := otel.GetTextMapPropagator()
	defer func() {
		otel.SetTracerProvider(prevTracer)
		otel.SetMeterProvider(prevMeter)
		otel.SetTextMapPropagator(prevProp)
	}()

	tracer := sdktrace.NewTracerProvider()
	meter := sdkmetric.NewMeterProvider()

	installGlobals(globalDeps{TracerProvider: tracer, MeterProvider: meter})

	if got := otel.GetTracerProvider(); got != tracer {
		t.Fatalf("expected tracer provider to be installed")
	}
	if got := otel.GetMeterProvider(); got != meter {
		t.Fatalf("expected meter provider to be installed")
	}
	fields := otel.GetTextMapPropagator().Fields()
	if !contains(fields, "traceparent") || !contains(fields, "baggage") {
		t.Fatalf("unexpected propagator fields %v", fields)
	}
}

func TestNewProvidersDisabled(t *testing.T) {
	core, logs := observer.New(zapcore.InfoLevel)
	logger := zap.New(core)
	disabled := true
	cfg := &Config{
		ServiceName:    "svc",
		ServiceVersion: "v1",
		Environment:    "test",
		Disabled:       &disabled,
	}
	ctx := context.Background()

	res, err := NewProviders(ctx, cfg, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.TracerProvider == nil || res.MeterProvider == nil {
		t.Fatalf("expected providers when disabled")
	}
	if res.Tracer == nil || res.Meter == nil {
		t.Fatalf("expected tracer and meter when disabled")
	}
	if cfg.TracingEnabled == nil || *cfg.TracingEnabled {
		t.Fatalf("expected tracing disabled")
	}
	if cfg.MetricsEnabled == nil || *cfg.MetricsEnabled {
		t.Fatalf("expected metrics disabled")
	}
	if logs.FilterMessage("telemetry disabled").Len() != 1 {
		t.Fatalf("expected disabled log entry")
	}
}

func TestNewProvidersWarnsWhenNoEndpoint(t *testing.T) {
	core, logs := observer.New(zapcore.WarnLevel)
	logger := zap.New(core)
	disabled := false
	tracing := true
	metrics := true
	cfg := &Config{
		ServiceName:    "svc",
		ServiceVersion: "v1",
		Environment:    "test",
		Disabled:       &disabled,
		TracingEnabled: &tracing,
		MetricsEnabled: &metrics,
	}
	ctx := context.Background()

	res, err := NewProviders(ctx, cfg, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.TracerProvider == nil || res.MeterProvider == nil {
		t.Fatalf("expected providers to be configured")
	}
	if logs.FilterMessage("tracing enabled but no OTLP endpoint set").Len() != 1 {
		t.Fatalf("expected tracing warning")
	}
	if logs.FilterMessage("metrics enabled but no OTLP endpoint set").Len() != 1 {
		t.Fatalf("expected metrics warning")
	}
}

func TestModuleReturnsOption(t *testing.T) {
	if Module() == nil {
		t.Fatalf("expected module option")
	}
}

func TestApplyConfigDefaults(t *testing.T) {
	origMeta := snapshotInfo()
	defer restoreInfo(origMeta)
	info.Name = "meta-name"
	info.Version = "meta-version"

	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "collector:4317")
	t.Setenv("OTEL_SERVICE_NAME", "env-service")
	t.Setenv("OTEL_SDK_DISABLED", "true")
	t.Setenv("ENV", "prod")

	cfg := &Config{}
	applyConfigDefaults(cfg)

	if cfg.OTLPEndpoint != "collector:4317" {
		t.Fatalf("unexpected endpoint: %s", cfg.OTLPEndpoint)
	}
	if cfg.ServiceName != "env-service" {
		t.Fatalf("unexpected service name: %s", cfg.ServiceName)
	}
	if cfg.ServiceVersion != "meta-version" {
		t.Fatalf("unexpected service version: %s", cfg.ServiceVersion)
	}
	if cfg.Environment != "prod" {
		t.Fatalf("unexpected environment: %s", cfg.Environment)
	}
	if cfg.Disabled == nil || !*cfg.Disabled {
		t.Fatalf("expected sdk disabled")
	}
	if cfg.TracingEnabled == nil || *cfg.TracingEnabled {
		t.Fatalf("expected tracing disabled")
	}
	if cfg.MetricsEnabled == nil || *cfg.MetricsEnabled {
		t.Fatalf("expected metrics disabled")
	}
	if cfg.TraceSampleRate != 1.0 {
		t.Fatalf("expected default trace sample rate")
	}
	if cfg.ExportInterval != 30*time.Second {
		t.Fatalf("expected default export interval, got %s", cfg.ExportInterval)
	}
}

func TestBuildResourceIncludesAttributes(t *testing.T) {
	origMeta := snapshotInfo()
	defer restoreInfo(origMeta)
	info.Name = ""
	info.Version = ""
	info.Description = ""
	info.Commit = ""
	info.Date = ""
	info.BuiltBy = ""
	info.GoVersion = ""

	disabled := true
	cfg := Config{
		ServiceName:        "svc",
		ServiceVersion:     "v1",
		Environment:        "qa",
		Disabled:           &disabled,
		ResourceAttributes: map[string]string{"extra.key": "extra"},
	}

	res, err := buildResource(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	attrs := res.Attributes()
	if !attrEquals(attrs, semconv.ServiceNameKey, "svc") {
		t.Fatalf("missing service name attribute")
	}
	if !attrEquals(attrs, semconv.ServiceVersionKey, "v1") {
		t.Fatalf("missing service version attribute")
	}
	if !attrEquals(attrs, semconv.DeploymentEnvironmentNameKey, "qa") {
		t.Fatalf("missing environment attribute")
	}
	if !boolAttrEquals(attrs, attribute.Key("otel.sdk.disabled"), true) {
		t.Fatalf("missing disabled attribute")
	}
	if !attrEquals(attrs, attribute.Key("extra.key"), "extra") {
		t.Fatalf("missing extra attribute")
	}
}

func TestCoalesceEnv(t *testing.T) {
	t.Setenv("FIRST", "")
	t.Setenv("SECOND", "value")
	t.Setenv("THIRD", "ignored")

	if got := coalesceEnv("FIRST", "SECOND", "THIRD"); got != "value" {
		t.Fatalf("expected SECOND, got %s", got)
	}
	if got := coalesceEnv("MISSING"); got != "" {
		t.Fatalf("expected empty string, got %s", got)
	}
}

func TestSetDefaultBool(t *testing.T) {
	var target *bool
	setDefaultBool(&target, true)
	if target == nil || !*target {
		t.Fatalf("expected pointer set to true")
	}
	setDefaultBool(&target, false)
	if !*target {
		t.Fatalf("existing pointer should remain unchanged")
	}
}

func TestRegisterShutdown(t *testing.T) {
	core, logs := observer.New(zapcore.InfoLevel)
	logger := zap.New(core)
	lc := fxtest.NewLifecycle(t)
	params := shutdownDeps{
		TracerProvider: sdktrace.NewTracerProvider(),
		MeterProvider:  sdkmetric.NewMeterProvider(),
		Logger:         logger,
		LC:             lc,
	}

	registerShutdown(params)
	ctx := context.Background()
	if err := lc.Start(ctx); err != nil {
		t.Fatalf("start lifecycle: %v", err)
	}
	if err := lc.Stop(ctx); err != nil {
		t.Fatalf("stop lifecycle: %v", err)
	}
	if logs.FilterMessage("shutting down telemetry providers").Len() != 1 {
		t.Fatalf("expected shutdown log entry")
	}
}

func TestBuildTracerProviderInvalidSampler(t *testing.T) {
	tracing := true
	cfg := Config{
		TracingEnabled:  &tracing,
		TraceSampler:    "invalid",
		TraceSampleRate: 1,
	}
	res := sdkresource.NewSchemaless()
	if _, err := buildTracerProvider(context.Background(), cfg, res); err == nil {
		t.Fatalf("expected sampler error")
	}
}

func TestBuildTracerProviderWithEndpoint(t *testing.T) {
	tracing := true
	cfg := Config{
		TracingEnabled:  &tracing,
		TraceSampleRate: 1,
		OTLPEndpoint:    "localhost:43179",
		Insecure:        true,
	}
	res := sdkresource.NewSchemaless()
	tp, err := buildTracerProvider(context.Background(), cfg, res)
	if err != nil {
		t.Fatalf("unexpected tracer provider error: %v", err)
	}
	if tp == nil {
		t.Fatalf("expected tracer provider instance")
	}
}

func TestShutdownHelpers(t *testing.T) {
	if err := shutdownTracer(context.Background(), nil, zap.NewNop()); err != nil {
		t.Fatalf("unexpected tracer nil error: %v", err)
	}
	if err := shutdownMeter(context.Background(), nil, zap.NewNop()); err != nil {
		t.Fatalf("unexpected meter nil error: %v", err)
	}
	if err := shutdownTracer(context.Background(), sdktrace.NewTracerProvider(), zap.NewNop()); err != nil {
		t.Fatalf("unexpected tracer shutdown error: %v", err)
	}
	if err := shutdownMeter(context.Background(), sdkmetric.NewMeterProvider(), zap.NewNop()); err != nil {
		t.Fatalf("unexpected meter shutdown error: %v", err)
	}
}

type infoSnapshot struct {
	name        string
	description string
	version     string
	commit      string
	date        string
	builtBy     string
	goVersion   string
}

func snapshotInfo() infoSnapshot {
	return infoSnapshot{
		name:        info.Name,
		description: info.Description,
		version:     info.Version,
		commit:      info.Commit,
		date:        info.Date,
		builtBy:     info.BuiltBy,
		goVersion:   info.GoVersion,
	}
}

func restoreInfo(s infoSnapshot) {
	info.Name = s.name
	info.Description = s.description
	info.Version = s.version
	info.Commit = s.commit
	info.Date = s.date
	info.BuiltBy = s.builtBy
	info.GoVersion = s.goVersion
}

func contains(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}

func attrEquals(attrs []attribute.KeyValue, key attribute.Key, value string) bool {
	for _, kv := range attrs {
		if kv.Key == key {
			return kv.Value.AsString() == value
		}
	}
	return false
}

func boolAttrEquals(attrs []attribute.KeyValue, key attribute.Key, expected bool) bool {
	for _, kv := range attrs {
		if kv.Key == key {
			return kv.Value.AsBool() == expected
		}
	}
	return false
}
