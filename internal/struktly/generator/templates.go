package generator

const MainTemplate = `package main

import (
	"log"
	"net/http"
)

func main() {
	mux := http.NewServeMux()
{{- range .Handlers }}
	mux.HandleFunc("{{ .Path }}", {{ .Name }})
{{- end }}

	log.Println("Starting {{ .Name }} on :8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err)
	}
}
`

const HandlersTemplate = `package main

import (
	"net/http"
)
{{ range .Handlers }}
// {{ .Name }} handles {{ .Method }} {{ .Path }}
// {{ .Description }}
func {{ .Name }}(w http.ResponseWriter, r *http.Request) {
	// AI_LOGIC_START: {{ .Name }}
	// AI_LOGIC_END: {{ .Name }}
}
{{ end }}
`

const TestTemplate = `package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)
{{ range .Handlers }}
func Test{{ .Name }}(t *testing.T) {
	// AI_TEST_START: {{ .Name }}
	// AI_TEST_END: {{ .Name }}
}
{{ end }}
`
