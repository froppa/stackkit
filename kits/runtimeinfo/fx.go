// Package runtimeinfo provides build-time metadata injected via -ldflags.
//
// It offers a standard way to embed version, commit, and other build information
// into a Go binary and expose it in common formats for observability.
//
// # Example usage in build scripts
//
//	go build -ldflags "\
//	  -X github.com/froppa/stackkit/kits/runtimeinfo.Version=1.2.3 \
//	  -X github.com/froppa/stackkit/kits/runtimeinfo.Commit=sha256 \
//	  -X github.com/froppa/stackkit/kits/runtimeinfo.BuiltBy=ci \
//	  -X github.com/froppa/stackkit/kits/runtimeinfo.Date=2025-07-15T12:00:00Z \
//	  -X github.com/froppa/stackkit/kits/runtimeinfo.GoVersion=go1.21.4 \
//	  -X github.com/froppa/stackkit/kits/runtimeinfo.Name=your-service-name \
//	  -X github.com/froppa/stackkit/kits/runtimeinfo.Description='Your service description'"
package runtimeinfo

import (
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	// Name is the service or binary name. Injected at build time.
	Name string

	// Description is a human-readable description of the service. Optional.
	Description string

	// Version is the semantic version of the build (e.g. v1.0.0).
	// Defaults to "dev" if unset.
	Version = "dev"

	// Commit is the Git commit hash of the build.
	Commit string

	// Date is the UTC timestamp when the binary was built.
	Date string

	// BuiltBy is the name of the builder (e.g. CI system or developer).
	BuiltBy string

	// GoVersion is the Go toolchain version used to compile the binary.
	GoVersion string
)

// Meta contains the full build metadata for introspection or logging.
type Meta struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     string `json:"version"`
	Commit      string `json:"git_commit"`
	Date        string `json:"build_time"`
	BuiltBy     string `json:"built_by"`
	GoVersion   string `json:"go_version"`
}

// GetMetadata returns a snapshot of the current build metadata.
func GetMetadata() Meta {
	return Meta{
		Name:        Name,
		Description: Description,
		Version:     Version,
		Commit:      Commit,
		Date:        Date,
		BuiltBy:     BuiltBy,
		GoVersion:   GoVersion,
	}
}

// Fields returns the build metadata as zap fields for structured logging.
// Useful for injecting metadata into root loggers.
func Fields() []zapcore.Field {
	return []zapcore.Field{
		zap.String("name", Name),
		zap.String("description", Description),
		zap.String("version", Version),
		zap.String("commit", Commit),
		zap.String("build_date", Date),
		zap.String("built_by", BuiltBy),
		zap.String("go_version", GoVersion),
	}
}

// OTELAttributes returns the build metadata as OpenTelemetry resource attributes.
// These attributes align with OpenTelemetry semantic conventions where applicable,
// making them automatically understandable by observability platforms.
func OTELAttributes() []attribute.KeyValue {
	m := GetMetadata()
	// Conditionally add attributes to avoid empty strings for unset optional fields.
	attrs := make([]attribute.KeyValue, 0, 7)

	if m.Name != "" {
		attrs = append(attrs, semconv.ServiceNameKey.String(m.Name))
	}
	if m.Version != "" {
		attrs = append(attrs, semconv.ServiceVersionKey.String(m.Version))
	}
	if m.Description != "" {
		// This is a custom attribute, but useful.
		attrs = append(attrs, attribute.String("service.description", m.Description))
	}
	if m.Commit != "" {
		// Custom attribute for source control revision.
		attrs = append(attrs, attribute.String("vcs.revision", m.Commit))
	}
	if m.GoVersion != "" {
		// Standard semantic convention for runtime version.
		attrs = append(attrs, semconv.ProcessRuntimeVersionKey.String(m.GoVersion))
	}
	if m.Date != "" {
		attrs = append(attrs, attribute.String("build.time", m.Date))
	}
	if m.BuiltBy != "" {
		attrs = append(attrs, attribute.String("build.user", m.BuiltBy))
	}
	return attrs
}

// PrometheusLabelKeys returns a stable list of label keys for Prometheus metrics.
// This ensures deterministic ordering when registering metrics with constant labels.
func PrometheusLabelKeys() []string {
	return []string{
		"name",
		"version",
		"commit",
		"built_by",
		"build_date",
		"go_version",
		"description",
	}
}

// PrometheusLabelValues returns the current values for each Prometheus label key,
// in the same order as returned by PrometheusLabelKeys().
func PrometheusLabelValues() []string {
	return []string{
		Name,
		Version,
		Commit,
		BuiltBy,
		Date,
		GoVersion,
		Description,
	}
}
