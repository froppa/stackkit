package policy

import (
	"fmt"
	"os"

	"github.com/froppa/stackkit/internal/struktly/ir"
	"gopkg.in/yaml.v3"
)

type Policy struct {
	MaxHandlers int      `yaml:"max_handlers"`
	Required    []string `yaml:"required"` // e.g. "tests"
}

func Load(path string) (*Policy, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var p Policy
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func Check(p *Policy, svc *ir.Service) error {
	if p.MaxHandlers > 0 && len(svc.Handlers) > p.MaxHandlers {
		return fmt.Errorf("policy violation: too many handlers (%d > %d)", len(svc.Handlers), p.MaxHandlers)
	}
	return nil
}
