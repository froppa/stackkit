package policy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/froppa/stackkit/internal/struktly/ir"
	"github.com/stretchr/testify/require"
)

const fixture = `max_handlers: 2
required:
  - tests
`

// loadFixture writes the policy fixture into a temp dir and loads it with Load.
func loadFixture(t *testing.T) *Policy {
	t.Helper()
	path := filepath.Join(t.TempDir(), "policy.yaml")
	require.NoError(t, os.WriteFile(path, []byte(fixture), 0o600))

	p, err := Load(path)
	require.NoError(t, err)
	require.Equal(t, 2, p.MaxHandlers)
	require.Equal(t, []string{"tests"}, p.Required)
	return p
}

func TestCheckWithinLimit(t *testing.T) {
	p := loadFixture(t)

	for _, n := range []int{1, 2} {
		svc := &ir.Service{Name: "svc", Handlers: make([]ir.Handler, n)}
		require.NoError(t, Check(p, svc), "handlers=%d", n)
	}
}

func TestCheckTooManyHandlers(t *testing.T) {
	p := loadFixture(t)
	svc := &ir.Service{Name: "svc", Handlers: make([]ir.Handler, 3)}

	err := Check(p, svc)
	require.Error(t, err)
	require.ErrorContains(t, err, "policy violation: too many handlers")
}
