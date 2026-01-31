package ir

import (
	"fmt"
	"os"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

// Simplified structures for parsing YAML
type openApiSpec struct {
	Info struct {
		Title string `yaml:"title"`
	} `yaml:"info"`
	Paths map[string]map[string]struct {
		Summary     string `yaml:"summary"`
		OperationID string `yaml:"operationId"`
	} `yaml:"paths"`
}

func Parse(filepath string) (*Service, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to read spec: %w", err)
	}

	var spec openApiSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("failed to parse yaml: %w", err)
	}

	svc := &Service{
		Name: spec.Info.Title,
	}

	// Normalize name
	svc.Name = strings.ReplaceAll(svc.Name, " ", "")

	for path, methods := range spec.Paths {
		for method, op := range methods {
			h := Handler{
				Name:        op.OperationID,
				Method:      strings.ToUpper(method),
				Path:        path,
				Description: op.Summary,
			}
			if h.Name == "" {
				// Fallback name if operationId is missing
				cleanPath := strings.ReplaceAll(path, "/", "_")
				cleanPath = strings.Trim(cleanPath, "_")
				h.Name = fmt.Sprintf("%s_%s", h.Method, cleanPath)
			}
			h.Name = sanitize(h.Name)
			svc.Handlers = append(svc.Handlers, h)
		}
	}

	return svc, nil
}

func sanitize(s string) string {
	// Simple camel casing for identifiers
	// e.g. list-pets -> ListPets
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	for i, p := range parts {
		if len(p) > 0 {
			r := []rune(p)
			r[0] = unicode.ToUpper(r[0])
			parts[i] = string(r)
		}
	}
	return strings.Join(parts, "")
}
