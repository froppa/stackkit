package configkit_test

import (
	"bytes"
	"testing"

	config "github.com/froppa/stackkit/kits/configkit"
	pkghttp "github.com/froppa/stackkit/kits/httpkit"
	"github.com/stretchr/testify/require"
	uber "go.uber.org/config"
)

func providerFromYAML(t *testing.T, y string) *uber.YAML {
	t.Helper()
	p, err := uber.NewYAML(uber.Source(bytes.NewBufferString(y)))
	require.NoError(t, err)
	return p
}

func TestDiscovery_ListAndCheck(t *testing.T) {
	config.ResetDiscoveryForTests()

	// Register requirement via ProvideFromKey usage.
	_ = config.ProvideFromKey[pkghttp.Config]("http")

	reqs := config.Requirements()
	require.Len(t, reqs, 1)
	require.Equal(t, "http", reqs[0].Key)

	// Missing required field should fail check.
	p1 := providerFromYAML(t, "http:\n  enable_pprof: true\n")
	res1 := config.Check(p1)
	require.Len(t, res1, 1)
	require.Equal(t, "http", res1[0].Key)
	require.False(t, res1[0].OK)
	require.Error(t, res1[0].Err)

	// Provide required field -> success.
	p2 := providerFromYAML(t, "http:\n  addr: \":8080\"\n")
	res2 := config.Check(p2)
	require.Len(t, res2, 1)
	require.True(t, res2[0].OK, "expected http config to validate")

	// Spec should indicate that 'addr' is required.
	fields, err := config.Spec(reqs[0])
	require.NoError(t, err)
	var hasAddr bool
	for _, f := range fields {
		if f.Path == "addr" && f.Required {
			hasAddr = true
			break
		}
	}
	require.True(t, hasAddr, "expected addr to be marked required in spec")
}
