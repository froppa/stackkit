# Telemetry Module

Pragmatic OpenTelemetry wiring for Fx applications.

## Features

- **Dependency Injection + Globals**: Exposes OTEL components (`Tracer`, `Meter`, etc.)
  via Fx _and_ installs them into the global OTEL registry so third-party libraries
  work without extra plumbing.
- **Automatic Configuration**: Loads settings from a YAML file key (default `"telemetry"`).
- **Standard Resource Identity**: Populates resources with service name, version, and
  environment using semantic conventions.
- **Safe Disabled Mode**: Honors `sdk.disabled` with true noop providers that preserve
  resource data and sampling semantics.
- **Export Readiness**: Warns when tracing or metrics are enabled without an OTLP
  endpoint so misconfigurations are visible at startup.
- **OTLP Exporters**: Automatically enables OTLP/gRPC trace and metric exporters
  if an endpoint is configured.
- **Configurable Sampling**: Allows trace sampling to be configured.
- **Graceful Shutdown**: Integrates with the Fx lifecycle for clean provider shutdown,
  ensuring telemetry data is flushed.

## Usage

Simply add `telemetry.Module()` to your `fx.New` constructor. This makes the OTEL
`trace.Tracer` and `metric.Meter` available for injection into your services and
updates the global OTEL providers.

```go
fx.New(
    configkit.Module(), // Your application's config module
    telemetry.Module(), // This OpenTelemetry module (installs providers globally)
    fx.Invoke(func(tracer trace.Tracer, meter metric.Meter) {
        // Now you can use the injected Tracer and Meter
        _, span := tracer.Start(context.Background(), "my-operation")
        defer span.End()
    }),
).Run()
```

## Configuration

The module follows a standard precedence order for configuration settings:

1. **Environment Variables**: Standard OTEL variables (e.g., `OTEL_EXPORTER_OTLP_ENDPOINT`,
   `OTEL_SERVICE_NAME`) have the highest precedence.
2. **YAML Configuration**: Values loaded from your `config.yml` file.
3. **Metadata Package**: Fallbacks for service name and version from the `runtimeinfo` package.
4. **Hardcoded Defaults**: Sensible defaults for any remaining values.

## Example `config.yml`

```yaml
telemetry:
  service_name: "my-auth-service"
  service_version: "1.2.3"
  environment: "production"
  otlp_endpoint: "otel-collector.observability:4317"
  insecure: false # Use true for local development without TLS
  tracing_enabled: true
  metrics_enabled: true
  trace_sampler: "parent_ratio"
  trace_sample_rate: 0.5 # Sample 50% of traces
  resource_attributes:
    team: "backend"

# Opting out

Applications that should not install the OTEL module can omit `telemetry.Module()` or
provide an alternative module in their Fx wiring (for example in `app/fx.go`).
```
