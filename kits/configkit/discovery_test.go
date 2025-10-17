package configkit_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	config "github.com/froppa/stackkit/kits/configkit"
	uber "go.uber.org/config"
)

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestNewYAML_EnvCONFIGWinsOverDefault(t *testing.T) {
	tmp := t.TempDir()
	cwd, _ := os.Getwd()
	_ = os.Chdir(tmp)
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	// default config/config.yml
	writeFile(t, filepath.Join("config", "config.yml"), []byte("foo: default\n"))

	// env file
	envFile := filepath.Join(tmp, "env.yml")
	writeFile(t, envFile, []byte("foo: env\n"))
	t.Setenv("CONFIG", envFile)

	p, err := config.NewYAML(context.Background())
	if err != nil {
		t.Fatalf("NewYAML error: %v", err)
	}
	var out struct {
		Foo string `yaml:"foo"`
	}
	if err := p.Get(uber.Root).Populate(&out); err != nil {
		t.Fatalf("populate: %v", err)
	}
	if out.Foo != "env" {
		t.Fatalf("expected env override, got %q", out.Foo)
	}
}

func TestNewYAML_DefaultUsedWhenEnvUnset(t *testing.T) {
	tmp := t.TempDir()
	cwd, _ := os.Getwd()
	_ = os.Chdir(tmp)
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	// default config/config.yml
	writeFile(t, filepath.Join("config", "config.yml"), []byte("foo: default\n"))

	p, err := config.NewYAML(context.Background())
	if err != nil {
		t.Fatalf("NewYAML error: %v", err)
	}
	var out struct {
		Foo string `yaml:"foo"`
	}
	if err := p.Get(uber.Root).Populate(&out); err != nil {
		t.Fatalf("populate: %v", err)
	}
	if out.Foo != "default" {
		t.Fatalf("expected default, got %q", out.Foo)
	}
}

func TestNewYAML_InvalidEnvPathErrors(t *testing.T) {
	tmp := t.TempDir()
	cwd, _ := os.Getwd()
	_ = os.Chdir(tmp)
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	t.Setenv("CONFIG", filepath.Join(tmp, "missing.yml"))
	if _, err := config.NewYAML(context.Background()); err == nil {
		t.Fatalf("expected error for missing CONFIG file")
	}
}

func TestNewYAML_CLIFlagBeatsEnv(t *testing.T) {
	tmp := t.TempDir()
	cwd, _ := os.Getwd()
	_ = os.Chdir(tmp)
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	// env file
	envFile := filepath.Join(tmp, "env.yml")
	writeFile(t, envFile, []byte("foo: env\n"))
	t.Setenv("CONFIG", envFile)

	// CLI file (highest precedence)
	cliFile := filepath.Join(tmp, "cli.yml")
	writeFile(t, cliFile, []byte("foo: cli\n"))

	p, err := config.NewYAML(context.Background(), config.WithSources(config.File(cliFile)))
	if err != nil {
		t.Fatalf("NewYAML error: %v", err)
	}
	var out struct {
		Foo string `yaml:"foo"`
	}
	if err := p.Get(uber.Root).Populate(&out); err != nil {
		t.Fatalf("populate: %v", err)
	}
	if out.Foo != "cli" {
		t.Fatalf("expected cli override, got %q", out.Foo)
	}
}
