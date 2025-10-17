package runtimeinfo_test

import (
	"testing"

	info "github.com/froppa/stackkit/kits/runtimeinfo"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

// TestDefaultVersion verifies that the package-level variables have the correct
// default values when no ldflags are injected. Specifically, it checks that
// Version defaults to "dev".
func TestDefaultVersion(t *testing.T) {
	// In a clean state (without setting any variables), Version should be "dev".
	require.Equal(t, "dev", info.Version, "Default version should be 'dev'")

	// Other variables should be their zero value (empty string).
	require.Equal(t, "", info.Name, "Default name should be an empty string")
}

// TestMetadata comprehensively tests all helper functions when build-time
// variables are populated.
func TestMetadata(t *testing.T) {
	// --- Setup: Save original state and inject test data ---
	// This is crucial for test isolation, as we are modifying package-level variables.
	originalName := info.Name
	originalDesc := info.Description
	originalVersion := info.Version
	originalCommit := info.Commit
	originalDate := info.Date
	originalBuiltBy := info.BuiltBy
	originalGoVersion := info.GoVersion

	// Use defer to guarantee that the original state is restored after the test runs.
	defer func() {
		info.Name = originalName
		info.Description = originalDesc
		info.Version = originalVersion
		info.Commit = originalCommit
		info.Date = originalDate
		info.BuiltBy = originalBuiltBy
		info.GoVersion = originalGoVersion
	}()

	// Inject specific test values into the package-level variables.
	info.Name = "test-service"
	info.Description = "A service for testing"
	info.Version = "v1.2.3"
	info.Commit = "abcdef123"
	info.Date = "2025-10-03T19:15:00Z"
	info.BuiltBy = "test-runner"
	info.GoVersion = "go1.99.9"

	// --- Run Sub-tests for each function ---

	t.Run("GetMetadata", func(t *testing.T) {
		m := info.GetMetadata()
		require.Equal(t, "test-service", m.Name)
		require.Equal(t, "A service for testing", m.Description)
		require.Equal(t, "v1.2.3", m.Version)
		require.Equal(t, "abcdef123", m.Commit)
		require.Equal(t, "2025-10-03T19:15:00Z", m.Date)
		require.Equal(t, "test-runner", m.BuiltBy)
		require.Equal(t, "go1.99.9", m.GoVersion)
	})

	t.Run("Fields", func(t *testing.T) {
		fields := info.Fields()
		fieldMap := make(map[string]string)
		for _, f := range fields {
			fieldMap[f.Key] = f.String
		}

		require.Equal(t, "test-service", fieldMap["name"])
		require.Equal(t, "A service for testing", fieldMap["description"])
		require.Equal(t, "v1.2.3", fieldMap["version"])
		require.Equal(t, "abcdef123", fieldMap["commit"])
		require.Equal(t, "2025-10-03T19:15:00Z", fieldMap["build_date"])
		require.Equal(t, "test-runner", fieldMap["built_by"])
		require.Equal(t, "go1.99.9", fieldMap["go_version"])
	})

	t.Run("OTELAttributes with all values set", func(t *testing.T) {
		attrs := info.OTELAttributes()
		attrMap := make(map[attribute.Key]string)
		for _, a := range attrs {
			attrMap[a.Key] = a.Value.AsString()
		}

		require.Equal(t, "test-service", attrMap[semconv.ServiceNameKey])
		require.Equal(t, "v1.2.3", attrMap[semconv.ServiceVersionKey])
		require.Equal(t, "go1.99.9", attrMap[semconv.ProcessRuntimeVersionKey])
		require.Equal(t, "A service for testing", attrMap["service.description"])
		require.Equal(t, "abcdef123", attrMap["vcs.revision"])
		require.Equal(t, "2025-10-03T19:15:00Z", attrMap["build.time"])
		require.Equal(t, "test-runner", attrMap["build.user"])
	})

	t.Run("OTELAttributes with optional values empty", func(t *testing.T) {
		// Temporarily unset optional fields to test the conditional logic.
		info.Description = ""
		info.Commit = ""
		info.BuiltBy = ""

		attrs := info.OTELAttributes()
		attrMap := make(map[attribute.Key]string)
		for _, a := range attrs {
			attrMap[a.Key] = a.Value.AsString()
		}

		// Required fields should still be present.
		require.Equal(t, "test-service", attrMap[semconv.ServiceNameKey])
		require.Equal(t, "v1.2.3", attrMap[semconv.ServiceVersionKey])

		// Optional fields that were unset should be absent from the map.
		_, hasDesc := attrMap["service.description"]
		_, hasCommit := attrMap["vcs.revision"]
		_, hasBuiltBy := attrMap["build.user"]

		require.False(t, hasDesc, "Expected empty description to be omitted")
		require.False(t, hasCommit, "Expected empty commit to be omitted")
		require.False(t, hasBuiltBy, "Expected empty built_by to be omitted")

		// Restore for the next sub-test
		info.Description = "A service for testing"
		info.Commit = "abcdef123"
		info.BuiltBy = "test-runner"
	})

	t.Run("PrometheusLabels", func(t *testing.T) {
		keys := info.PrometheusLabelKeys()
		values := info.PrometheusLabelValues()

		require.Equal(t, len(keys), len(values), "Prometheus keys and values must have the same length")
		require.NotEmpty(t, keys, "Prometheus keys should not be empty")

		labelMap := make(map[string]string)
		for i, key := range keys {
			labelMap[key] = values[i]
		}

		require.Equal(t, "test-service", labelMap["name"])
		require.Equal(t, "v1.2.3", labelMap["version"])
		require.Equal(t, "abcdef123", labelMap["commit"])
		require.Equal(t, "test-runner", labelMap["built_by"])
		require.Equal(t, "2025-10-03T19:15:00Z", labelMap["build_date"])
		require.Equal(t, "go1.99.9", labelMap["go_version"])
		require.Equal(t, "A service for testing", labelMap["description"])
	})
}
