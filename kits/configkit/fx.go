// Package config provides a simple, tag-based configuration loader for an Fx application,
// built on uber/config and go-playground/validator.
package configkit

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/froppa/stackkit/kits/runtimeinfo"
	"github.com/go-playground/validator/v10"
	uber "go.uber.org/config"
	"go.uber.org/fx"
)

// validate is a singleton instance of the validator used for all config structs.
var validate = validator.New()

// Module wires the core uber/config YAML provider into an Fx application.
//
// This is the foundational component that enables configuration loading. It must be
// included in your Fx app for `Provide` and `ProvideFromKey` to work.
//
// Configuration is loaded with the following precedence (from lowest to highest,
// with later values overriding earlier ones):
// 1. Custom Sources: Provided via `WithSources()` or `WithEmbeddedBytes()`.
// 2. Base Config: `config/config.yml`
// 3. Local Overrides: `config/config.local.yml`
// 4. Service-Specific Overrides: `config/<service-name>.yml` (from the runtimeinfo package).
// 5. Environment Variables: Any `${...}` placeholders are expanded.
func Module(opts ...ModuleOption) fx.Option {
	var cfg moduleOpts
	for _, opt := range opts {
		opt(&cfg)
	}
	return fx.Provide(func() (*uber.YAML, error) {
		return load(cfg.extra...)
	})
}

// Provide returns an Fx provider that loads the entire configuration into type T,
// validates it, and provides a pointer to it (`*T`) to the Fx container.
//
// It is a convenient shorthand for `ProvideFromKey[T](uber.Root)`.
func Provide[T any]() func(*uber.YAML) (*T, error) {
	return ProvideFromKey[T](uber.Root)
}

// ProvideFromKey returns an Fx provider that loads a specific configuration
// subtree (identified by `key`) into type T, validates it, and provides a
// pointer to it (`*T`) to the Fx container.
//
// If validation fails based on the `validate` tags in the struct, the Fx
// application will fail to start with a descriptive error.
func ProvideFromKey[T any](key string) func(provider *uber.YAML) (*T, error) {
	// Register this requirement at construction time for discovery.
	registerRequirementFor[T](key)
	return func(provider *uber.YAML) (*T, error) {
		var cfg T
		if err := provider.Get(key).Populate(&cfg); err != nil {
			return nil, fmt.Errorf("config: could not populate key %q into %T: %w", key, cfg, err)
		}

		// Automatically run struct validation after populating.
		if err := validate.Struct(&cfg); err != nil {
			return nil, fmt.Errorf("config: validation failed for key %q (%T): %w", key, cfg, err)
		}

		return &cfg, nil
	}
}

// ModuleOption customizes the behavior of the config Module by adding extra sources.
type ModuleOption func(*moduleOpts)

// WithSources injects additional uber/config sources at the lowest precedence.
// This is useful for providing default configurations from code.
func WithSources(srcs ...uber.YAMLOption) ModuleOption {
	return func(o *moduleOpts) {
		o.extra = append(o.extra, srcs...)
	}
}

// WithEmbeddedBytes adds an embedded YAML payload (e.g., from `//go:embed`) as a
// low-precedence source for default values.
func WithEmbeddedBytes(b []byte) ModuleOption {
	return WithSources(uber.Source(bytes.NewReader(b)))
}

// --- Internal Implementation ---

type moduleOpts struct {
	extra []uber.YAMLOption
}

// load builds the layered uber/config provider from all available sources.
func load(extra ...uber.YAMLOption) (*uber.YAML, error) {
	// Pre-allocate slice with a reasonable capacity.
	opts := make([]uber.YAMLOption, 0, len(extra)+4)

	// Custom sources have the lowest precedence.
	opts = append(opts, extra...)

	// File-based sources are layered on top.
	opts = append(opts, fileOptions("config")...)

	// Environment variable expansion has the highest precedence.
	opts = append(opts, uber.Expand(os.LookupEnv))

	return uber.NewYAML(opts...)
}

// fileOptions discovers and returns YAML options for standard config file locations.
func fileOptions(dir string) []uber.YAMLOption {
	// Standard configuration files to search for, in order of precedence.
	files := []string{
		filepath.Join(dir, "config.yml"),       // Base config
		filepath.Join(dir, "config.local.yml"), // Local overrides
	}

	// Add a service-specific override file if the service name is set via runtimeinfo.
	// This allows for multi-service repos with shared base configs.
	if name := strings.TrimSpace(runtimeinfo.Name); name != "" {
		files = append(files, filepath.Join(dir, name+".yml"))
	}

	var opts []uber.YAMLOption
	for _, path := range files {
		// Only include the file source if it exists and is a regular file.
		if fi, err := os.Stat(path); err == nil && !fi.IsDir() {
			opts = append(opts, uber.File(path))
		}
	}
	return opts
}
