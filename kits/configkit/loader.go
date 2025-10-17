package configkit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	uber "go.uber.org/config"
)

// YAMLProvider is the concrete provider type used throughout the repo.
// It aliases the underlying uber/config YAML type for convenience.
type YAMLProvider = uber.YAML

// Source is an alias for uber/config YAML options (file, reader, expand, etc.).
type Source = uber.YAMLOption

// File returns a Source that loads YAML from the given path.
func File(path string) Source { return uber.File(path) }

// DefaultSources returns the default, low-precedence sources for CLI usage.
// Precedence (lowest -> highest) when combined by NewYAML:
//  1. Default file: config/config.yml (if present)
//  2. Env override: CONFIG=file.yml (if set, must exist)
//  3. CLI flag: passed via opts (highest precedence)
//
// Note: Services should continue using Module(); DefaultSources is intended for CLIs.
func DefaultSources() []Source {
	var out []Source
	// Default file (if present)
	if fi, err := os.Stat(filepath.Join("config", "config.yml")); err == nil && !fi.IsDir() {
		out = append(out, uber.File(filepath.Join("config", "config.yml")))
	}
	return out
}

// NewYAML builds a YAML provider using the same underlying primitives as Module,
// but with a CLI-friendly precedence model:
//
//	default config file -> $CONFIG override -> explicit sources via opts (highest)
//
// Environment expansion is always applied.
// If $CONFIG is set but the file is missing, an error is returned.
func NewYAML(_ context.Context, opts ...ModuleOption) (*YAMLProvider, error) {
	// Collect options via existing option type to avoid expanding API surface.
	var o moduleOpts
	for _, opt := range opts {
		opt(&o)
	}

	// Build precedence stack.
	// Start with default on-disk file if present.
	chain := make([]uber.YAMLOption, 0, 4)
	chain = append(chain, DefaultSources()...)

	// Env CONFIG override (must exist if set)
	if cfgPath, ok := os.LookupEnv("CONFIG"); ok {
		if fi, err := os.Stat(cfgPath); err == nil && !fi.IsDir() {
			chain = append(chain, uber.File(cfgPath))
		} else {
			return nil, fmt.Errorf("config: CONFIG path %q not found or not a file", cfgPath)
		}
	}

	// CLI-provided sources (highest precedence for CLIs)
	if len(o.extra) > 0 {
		chain = append(chain, o.extra...)
	}

	// Always expand environment variables.
	chain = append(chain, uber.Expand(os.LookupEnv))

	// Build provider.
	if len(chain) == 0 {
		return nil, errors.New("config: no configuration sources available")
	}
	return uber.NewYAML(chain...)
}
