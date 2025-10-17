package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/froppa/stackkit/kits/configkit"

	// Register known modules via init hooks so discovery/check commands
	// automatically pull in their configuration specs.
	_ "github.com/froppa/stackkit/kits/healthkit"
	_ "github.com/froppa/stackkit/kits/httpkit"
	_ "github.com/froppa/stackkit/kits/telemetry"
)

func main() {
	root := newRootCmd()
	if err := root.Execute(); err != nil {
		var exitErr *exitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.code)
		}
		if writeErr := writeln(root.ErrOrStderr(), err); writeErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "stackctl: %v\n", err)
		}
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:          "stackctl",
		Short:        "Utility toolbox for stackkit modules",
		SilenceUsage: true,
	}

	root.AddCommand(newConfigCmd())

	return root
}

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect, validate, and document configuration requirements",
	}

	cmd.AddCommand(newConfigCheckCmd())
	cmd.AddCommand(newConfigListCmd())
	cmd.AddCommand(newConfigDiscoveryCmd())

	return cmd
}

type exitError struct {
	code int
}

func (e *exitError) Error() string { return "" }

// --- config check ----------------------------------------------------------------

type configCheckOptions struct {
	key    string
	all    bool
	cfgRef string
}

func newConfigCheckCmd() *cobra.Command {
	opts := &configCheckOptions{}

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Validate configuration values for selected keys",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runConfigCheck(cmd, opts)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&opts.key, "key", "", "Configuration key to check (required unless --all is set)")
	flags.BoolVar(&opts.all, "all", false, "Validate every known configuration key")
	flags.StringVar(&opts.cfgRef, "config", "", "Path to YAML config file (highest precedence)")

	return cmd
}

func runConfigCheck(cmd *cobra.Command, opts *configCheckOptions) error {
	if err := validateCheckArgs(opts); err != nil {
		return err
	}

	keys, err := collectKeys(opts.key, opts.all)
	if err != nil {
		return err
	}

	// Register requirements for selected keys from the Known registry.
	for _, k := range keys {
		if t, ok := configkit.KnownType(k); ok {
			configkit.RegisterRequirementType(k, t)
		}
	}

	provider, err := loadProvider(cmd.Context(), opts.cfgRef)
	if err != nil {
		return err
	}

	results := configkit.Check(provider)
	selected := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		selected[k] = struct{}{}
	}

	out := cmd.OutOrStdout()
	exitCode := 0
	for _, r := range results {
		if _, ok := selected[r.Key]; !ok {
			continue
		}
		if r.OK {
			if err := writef(out, "[OK] %s\n", r.Key); err != nil {
				return err
			}
			continue
		}
		for _, issue := range r.Issues {
			if err := writef(out, "[ERROR] %s: %s\n", formatPath(r.Key, ""), issue); err != nil {
				return err
			}
			exitCode = 1
		}
		for _, unk := range r.Unknown {
			if err := writef(out, "[WARN] %s: unknown key %s\n", r.Key, unk); err != nil {
				return err
			}
		}
		if r.Err != nil && len(r.Issues) == 0 {
			if err := writef(out, "[ERROR] %s: %v\n", r.Key, r.Err); err != nil {
				return err
			}
			exitCode = 1
		}
	}

	if exitCode != 0 {
		return &exitError{code: exitCode}
	}
	return nil
}

func validateCheckArgs(opts *configCheckOptions) error {
	if opts.all {
		return nil
	}
	if opts.key == "" {
		return fmt.Errorf("--key is required unless --all is set")
	}
	return nil
}

func collectKeys(single string, all bool) ([]string, error) {
	if all {
		known := configkit.Known()
		keys := make([]string, 0, len(known))
		for _, r := range known {
			keys = append(keys, r.Key)
		}
		sort.Strings(keys)
		if len(keys) == 0 {
			return nil, fmt.Errorf("--all requested but no known configuration keys registered")
		}
		return keys, nil
	}
	return []string{single}, nil
}

// --- config list -----------------------------------------------------------------

type configListOptions struct {
	key         string
	format      string
	showSecrets bool
	cfgRef      string
}

func newConfigListCmd() *cobra.Command {
	opts := &configListOptions{}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "Render configuration values for a given key",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runConfigList(cmd, opts)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&opts.key, "key", "", "Configuration key to display (required)")
	flags.StringVar(&opts.format, "format", "yaml", "Output format: yaml|json")
	flags.BoolVar(&opts.showSecrets, "show-secrets", false, "Include secret values in output")
	flags.StringVar(&opts.cfgRef, "config", "", "Path to YAML config file (highest precedence)")

	return cmd
}

func runConfigList(cmd *cobra.Command, opts *configListOptions) error {
	if opts.key == "" {
		return fmt.Errorf("--key is required")
	}

	provider, err := loadProvider(cmd.Context(), opts.cfgRef)
	if err != nil {
		return err
	}

	var raw any
	if err := provider.Get(opts.key).Populate(&raw); err != nil {
		return err
	}
	var outVal any
	if opts.showSecrets {
		outVal = normalizeForPrint(raw)
	} else {
		outVal = configkit.Redact(opts.key, raw)
	}

	out := cmd.OutOrStdout()
	switch strings.ToLower(opts.format) {
	case "", "yaml":
		b, err := yaml.Marshal(outVal)
		if err != nil {
			return err
		}
		if err := write(out, string(b)); err != nil {
			return err
		}
	case "json":
		b, err := json.MarshalIndent(outVal, "", "  ")
		if err != nil {
			return err
		}
		if err := write(out, string(b)); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported format %q; use yaml or json", opts.format)
	}
	return nil
}

// --- config discovery -----------------------------------------------------------

type configDiscoveryOptions struct {
	cfgRef string
}

func newConfigDiscoveryCmd() *cobra.Command {
	opts := &configDiscoveryOptions{}
	cmd := &cobra.Command{
		Use:   "discovery",
		Short: "Show discovered configuration requirements and optional field specs",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runConfigDiscovery(cmd, opts)
		},
	}

	cmd.Flags().StringVar(&opts.cfgRef, "from-yaml", "", "Optional path to YAML file for unknown key detection")
	return cmd
}

func runConfigDiscovery(cmd *cobra.Command, opts *configDiscoveryOptions) error {
	known := configkit.Known()
	keys := make([]string, 0, len(known))
	for _, r := range known {
		if t, ok := configkit.KnownType(r.Key); ok {
			configkit.RegisterRequirementType(r.Key, t)
			keys = append(keys, r.Key)
		}
	}
	sort.Strings(keys)

	var (
		provider *configkit.YAMLProvider
		err      error
	)
	if opts.cfgRef != "" {
		provider, err = configkit.NewYAML(cmd.Context(), configkit.WithSources(configkit.File(opts.cfgRef)))
		if err != nil {
			return err
		}
	} else {
		provider, _ = configkit.NewYAML(cmd.Context())
	}

	out := cmd.OutOrStdout()
	if err := writeln(out, "Discovered configuration requirements:"); err != nil {
		return err
	}
	for _, req := range configkit.Requirements() {
		if err := writef(out, "- %s (%s)\n", req.Key, req.Type); err != nil {
			return err
		}
		specs, err := configkit.Spec(req)
		if err != nil {
			continue
		}
		for _, f := range specs {
			reqMark := ""
			if f.Required {
				reqMark = " (required)"
			}
			if err := writef(out, "    %s: %s%s\n", f.Path, f.Type, reqMark); err != nil {
				return err
			}
		}
	}

	if provider != nil {
		results := configkit.Check(provider)
		for _, r := range results {
			for _, unk := range r.Unknown {
				if err := writef(out, "[WARN] %s: unknown key %s\n", r.Key, unk); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// --- helpers --------------------------------------------------------------------

func loadProvider(ctx context.Context, cfgRef string) (*configkit.YAMLProvider, error) {
	if cfgRef == "" {
		return configkit.NewYAML(ctx)
	}
	return configkit.NewYAML(ctx, configkit.WithSources(configkit.File(cfgRef)))
}

func formatPath(key, path string) string {
	if path == "" {
		return key
	}
	return key + "." + path
}

func normalizeForPrint(v any) any {
	switch t := v.(type) {
	case map[any]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[fmt.Sprint(k)] = normalizeForPrint(val)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k] = normalizeForPrint(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = normalizeForPrint(val)
		}
		return out
	default:
		return t
	}
}

func writef(w io.Writer, format string, args ...any) error {
	_, err := fmt.Fprintf(w, format, args...)
	return err
}

func writeln(w io.Writer, args ...any) error {
	_, err := fmt.Fprintln(w, args...)
	return err
}

func write(w io.Writer, s string) error {
	_, err := fmt.Fprint(w, s)
	return err
}
