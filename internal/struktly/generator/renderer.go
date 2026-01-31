package generator

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/froppa/stackkit/internal/struktly/ir"
)

func Render(svc *ir.Service) (map[string]string, error) {
	files := make(map[string]string)

	// Render Main
	mainContent, err := renderTemplate("main", MainTemplate, svc)
	if err != nil {
		return nil, err
	}
	files["main.go"] = mainContent

	// Render Handlers
	handlersContent, err := renderTemplate("handlers", HandlersTemplate, svc)
	if err != nil {
		return nil, err
	}
	files["handlers.go"] = handlersContent

	// Render Tests
	testContent, err := renderTemplate("main_test", TestTemplate, svc)
	if err != nil {
		return nil, err
	}
	files["main_test.go"] = testContent

	return files, nil
}

func renderTemplate(name, tmplStr string, data interface{}) (string, error) {
	tmpl, err := template.New(name).Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse template %s: %w", name, err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template %s: %w", name, err)
	}
	return buf.String(), nil
}
