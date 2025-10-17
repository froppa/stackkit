# Config Module

Simple, tag-based configuration loader for Fx applications.
Built on top of **uber/config** and **go-playground/validator**.

## Features

- **Layered YAML Loading**: Automatically loads and merges multiple YAML files.
- **Environment Variable Expansion**: Supports `${VAR:default}` syntax in config files.
- **Automatic Validation**: Recursively validates structs using `validate` tags.
- **Fx Integration**: Provides strongly-typed configuration to the Fx container.
- **Sub-Tree Loading**: Easily provide nested parts of your configuration to different modules.

---

## Configuration Loading

The module loads configuration with the following precedence (from lowest to highest, with later values overriding earlier ones):

1. **Custom Sources**: Any sources provided via `configkit.WithSources()` or `configkit.WithEmbeddedBytes()`.
2. **Base Config**: `config/config.yml`
3. **Local Overrides**: `config/config.local.yml` (ideal for development, should be in `.gitignore`).
4. **Service-Specific Overrides**: `config/<service-name>.yml` (uses the name from the runtimeinfo package).
5. **Environment Variables**: Any `${...}` placeholders are expanded.

### CLI-oriented loader

For tooling and one-off inspection, `configkit.NewYAML` provides a minimal loader that reuses the same internals but applies a simpler precedence geared towards CLIs:

- Default file: `config/config.yml` (if present)
- Env override: `CONFIG=/path/to/file.yml` (must exist)
- CLI flag: pass an explicit file via `configkit.WithSources(configkit.File(path))` (highest precedence)

The CLI loader always applies environment expansion and never logs secrets. Use `configkit.Redact(key, value)` to render a redacted view for display.

On Fx boot via `configfx.Module`, a single line is emitted:

```
msg="config.loaded" env="$ENV" service="$runtimeinfo.Name" version="$runtimeinfo.Version"
```

Only these fields are logged; configuration values are never included.

---

## Usage

### 1. Define Your Configuration Structs

Create Go structs that mirror your YAML structure. Use `yaml` tags for mapping and `validate` tags for validation rules.

```go
// myapp/config.go

// HTTPConfig defines the settings for the web server.
type HTTPConfig struct {
  Addr string `yaml:"addr" validate:"required,hostname_port"`
}

// AppConfig is the root configuration for the application.
type AppConfig struct {
  HTTP HTTPConfig `yaml:"http"`
  // ... other configs
}
```

---

### 2. Provide Configuration in Fx

In your `main.go`, add `configkit.Module()` to enable the loader. Then, use a `configkit.Provide` function to load, validate, and provide your typed config struct to the application.

#### Provide the entire config

```go
// main.go
app := fx.New(
	// 1. Enable the configuration loader.
	configkit.Module(),

	// 2. Load the YAML into AppConfig, validate it, and provide *AppConfig.
	fx.Provide(configkit.Provide[AppConfig]()),

	// 3. Now you can inject *AppConfig anywhere.
	fx.Invoke(func(cfg *AppConfig) {
		fmt.Println("HTTP server will run on:", cfg.HTTP.Addr)
	}),
)
```

#### Provide a specific sub-tree

This is useful for modularity. A module can ask for just the part of the config it needs.

```go
// http/module.go
func Module() fx.Option {
	return fx.Options(
		// This provider loads only the "http" key from the YAML
		// into an HTTPConfig struct and provides *HTTPConfig.
		fx.Provide(configkit.ProvideFromKey[HTTPConfig]("http")),
		// ... other providers that can now inject *HTTPConfig
	)
}
```

---

## Advanced Usage

### Adding Custom Configuration Sources

You can add extra configuration sources, like an embedded default config, at the lowest precedence.

```go
import "embed"

//go:embed defaults.yml
var default_config []byte

func main() {
  app := fx.New(
    configkit.Module(
      // This embedded config will be loaded first.
      configkit.WithEmbeddedBytes(default_config),
      // You can also add other custom files.
      configkit.WithSources(uber.File("another_config.yml")),
    ),
    // ... other providers
  )
}
```

### Config Discovery and Validation

This package can automatically discover which config subtrees your app uses and validate them.

- Discovery is triggered whenever you call `configkit.ProvideFromKey[T](key)` or `configkit.Provide[T]()`.
- At runtime, you can list and validate the discovered requirements:

```go
// List discovered requirements
reqs := configkit.Requirements()

// Validate them against the current provider
results := configkit.Check(provider)
for _, r := range results {
  if !r.OK {
    log.Error("config invalid", zap.String("key", r.Key), zap.String("type", r.Type), zap.Error(r.Err))
  }
}

// Optionally, get a field spec for documentation (uses yaml/json + validate tags)
fields, _ := configkit.Spec(reqs[0])
```

CLI helper (optional):

```bash
go run github.com/froppa/stackkit/cmd/stackctl config check --all
```

Notes:
- The CLI registers modules you pass via `--with`.
- Field specs use `yaml` tags primarily and fall back to `json`. Required is inferred from `validate:"required"`.
