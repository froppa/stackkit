package ir

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	// One path and one method: spec.Paths is a map, so handler order is not deterministic.
	spec := `info:
  title: Pet Store API
paths:
  /pets:
    get:
      summary: List all pets
      operationId: list-pets
`
	path := filepath.Join(t.TempDir(), "openapi.yaml")
	require.NoError(t, os.WriteFile(path, []byte(spec), 0o600))

	svc, err := Parse(path)
	require.NoError(t, err)
	require.NotNil(t, svc)

	assert.Equal(t, "PetStoreAPI", svc.Name)
	require.Len(t, svc.Handlers, 1)
	assert.Equal(t, Handler{
		Name:        "ListPets",
		Method:      "GET",
		Path:        "/pets",
		Description: "List all pets",
	}, svc.Handlers[0])
}

func TestParseMissingFile(t *testing.T) {
	svc, err := Parse(filepath.Join(t.TempDir(), "missing.yaml"))
	require.Error(t, err)
	assert.Nil(t, svc)
}

func TestSanitize(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"list-pets", "ListPets"},
		{"get_pet_by_id", "GetPetById"},
		{"GET_v2/pets", "GETV2Pets"},
		{"listPets", "ListPets"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			assert.Equal(t, tt.want, sanitize(tt.in))
		})
	}
}
