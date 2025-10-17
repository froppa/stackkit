package configkit_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/froppa/stackkit/kits/configkit"
	info "github.com/froppa/stackkit/kits/runtimeinfo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	uberconfig "go.uber.org/config"
	"go.uber.org/fx"
)

func readFixture(t *testing.T, rel string) []byte {
	t.Helper()
	_, this, _, _ := runtime.Caller(0)
	base := filepath.Dir(this)
	p := filepath.Join(base, rel)
	b, err := os.ReadFile(p)
	require.NoErrorf(t, err, "read fixture %s", p)
	return b
}

func writeConfigFile(t *testing.T, path string, contents []byte) error {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, contents, 0o644)
}

func configFile(t *testing.T, b []byte) (*uberconfig.YAML, error) {
	t.Helper()
	return uberconfig.NewYAML(uberconfig.Source(bytes.NewReader(b)))
}

func startApp(t *testing.T, opts ...fx.Option) *fx.App {
	t.Helper()
	app := fx.New(opts...)
	require.NoError(t, app.Start(context.Background()))
	t.Cleanup(func() { _ = app.Stop(context.Background()) })
	return app
}

func TestModule_WithEmbeddedBytes(t *testing.T) {
	tmp := t.TempDir()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmp))
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	y := []byte("svc:\n  flag: true\n")

	type svcCfg struct {
		Svc struct {
			Flag bool `yaml:"flag"`
		} `yaml:"svc"`
	}

	var cfg svcCfg
	startApp(t,
		configkit.Module(configkit.WithEmbeddedBytes(y)),
		fx.Provide(configkit.Provide[svcCfg]()),
		fx.Invoke(func(c *svcCfg) { cfg = *c }),
	)

	assert.True(t, cfg.Svc.Flag)
}

func TestModule_ProvidesConfig(t *testing.T) {
	tmp := t.TempDir()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmp))
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	var got *uberconfig.YAML
	startApp(t,
		configkit.Module(),
		fx.Invoke(func(c *uberconfig.YAML) { got = c }),
	)

	require.NotNil(t, got)
}

func TestProvideFromKey_PrecedenceAndNested(t *testing.T) {
	tmp := t.TempDir()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmp))
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	prevName := info.Name
	info.Name = "test-service"
	t.Cleanup(func() { info.Name = prevName })

	require.NoError(t, writeConfigFile(t, filepath.Join("config", "config.yml"), readFixture(t, "testdata/config/config.yml")))
	require.NoError(t, writeConfigFile(t, filepath.Join("config", info.Name+".yml"), readFixture(t, "testdata/config/test-service.yml")))

	type metaConfig struct {
		Enabled bool `yaml:"enabled"`
	}
	type baseConfig struct {
		Foo    string `yaml:"foo" validate:"required"`
		Nested struct {
			Value int `yaml:"value" validate:"min=1"`
		} `yaml:"nested"`
		Meta metaConfig `yaml:"meta"`
	}

	var base baseConfig
	var mc metaConfig

	startApp(t,
		configkit.Module(),
		fx.Provide(configkit.Provide[baseConfig]()),
		fx.Provide(configkit.ProvideFromKey[metaConfig]("meta")),
		fx.Invoke(func(b *baseConfig, m *metaConfig) {
			base = *b
			mc = *m
		}),
	)

	assert.Equal(t, "bar", base.Foo)
	assert.Equal(t, 42, base.Nested.Value)
	assert.True(t, base.Meta.Enabled)
	assert.True(t, mc.Enabled)
}

func TestProvideFromKey_ValidationFailure(t *testing.T) {
	yml, err := configFile(t, []byte("svc:\n  port: 0\n"))
	require.NoError(t, err)

	type svcCfg struct {
		Port int `yaml:"port" validate:"min=1"`
	}

	provider := configkit.ProvideFromKey[svcCfg]("svc")
	got, perr := provider(yml)
	require.Error(t, perr)
	assert.Nil(t, got)
	assert.True(t, strings.Contains(perr.Error(), "validation failed"))
}

func TestModule_DefaultConfigDir(t *testing.T) {
	tmp := t.TempDir()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmp))
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	require.NoError(t, writeConfigFile(t, filepath.Join("config", "config.yml"), []byte("foo: hello\n")))

	type cfg struct {
		Foo string `yaml:"foo" validate:"required"`
	}

	var out cfg
	startApp(t,
		configkit.Module(),
		fx.Provide(configkit.Provide[cfg]()),
		fx.Invoke(func(c *cfg) { out = *c }),
	)

	assert.Equal(t, "hello", out.Foo)
}

func TestModule_WithSources_Precedence(t *testing.T) {
	tmp := t.TempDir()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmp))
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	svcSrc := uberconfig.Source(bytes.NewBufferString("foo: svc\nnested:\n  value: 1\n"))
	require.NoError(t, writeConfigFile(t, filepath.Join("config", "config.yml"), []byte("foo: file\nnested:\n  value: 2\n")))

	type cfg struct {
		Foo    string `yaml:"foo"`
		Nested struct {
			Value int `yaml:"value"`
		} `yaml:"nested"`
	}

	var out cfg
	startApp(t,
		configkit.Module(configkit.WithSources(svcSrc)),
		fx.Provide(configkit.Provide[cfg]()),
		fx.Invoke(func(c *cfg) { out = *c }),
	)

	assert.Equal(t, "file", out.Foo)
	assert.Equal(t, 2, out.Nested.Value)
}

func TestEnvExpansion_Overrides(t *testing.T) {
	tmp := t.TempDir()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmp))
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	yaml := []byte("http:\n  addr: ${APP_HTTP_ADDR:\":8080\"}\n")
	require.NoError(t, writeConfigFile(t, filepath.Join("config", "config.yml"), yaml))
	t.Setenv("APP_HTTP_ADDR", ":9999")

	type cfg struct {
		HTTP struct {
			Addr string `yaml:"addr" validate:"required"`
		} `yaml:"http"`
	}

	var out cfg
	startApp(t,
		configkit.Module(),
		fx.Provide(configkit.Provide[cfg]()),
		fx.Invoke(func(c *cfg) { out = *c }),
	)

	assert.Equal(t, ":9999", out.HTTP.Addr)
}
