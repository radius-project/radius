package sanitization

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOmitContainerEnvironment(t *testing.T) {
	t.Parallel()

	properties := map[string]any{
		"application": "app-id",
		"containers": map[string]any{
			"frontend": map[string]any{
				"image": "frontend:latest",
				"env": map[string]any{
					"PUBLIC_VALUE": "visible",
					"PASSWORD":     "secret",
				},
				"ports": map[string]any{"http": map[string]any{"containerPort": 3000}},
			},
			"worker": map[string]any{
				"image": "worker:latest",
				"env":   "unexpected-shape",
			},
			"empty": nil,
		},
	}

	result := OmitContainerEnvironment("radius.compute/CONTAINERS", properties)

	containers := result["containers"].(map[string]any)
	frontend := containers["frontend"].(map[string]any)
	worker := containers["worker"].(map[string]any)
	require.NotContains(t, frontend, "env")
	require.NotContains(t, worker, "env")
	require.Equal(t, "frontend:latest", frontend["image"])
	require.Contains(t, frontend, "ports")
	require.Equal(t, "app-id", result["application"])

	originalContainers := properties["containers"].(map[string]any)
	require.Contains(t, originalContainers["frontend"].(map[string]any), "env")
	require.Contains(t, originalContainers["worker"].(map[string]any), "env")
}

func TestOmitContainerEnvironmentPreservesOtherResourceTypes(t *testing.T) {
	t.Parallel()

	properties := map[string]any{
		"containers": map[string]any{
			"example": map[string]any{"env": map[string]any{"VALUE": "preserved"}},
		},
	}

	result := OmitContainerEnvironment("Contoso.Resources/widgets", properties)

	require.Equal(t, properties, result)
	resultContainers := result["containers"].(map[string]any)
	originalContainers := properties["containers"].(map[string]any)
	resultContainers["changed"] = true
	require.NotContains(t, originalContainers, "changed")
}

func TestOmitContainerEnvironmentHandlesNilAndMalformedContainers(t *testing.T) {
	t.Parallel()

	require.Nil(t, OmitContainerEnvironment("Radius.Compute/containers", nil))
	require.Equal(t,
		map[string]any{"containers": "malformed"},
		OmitContainerEnvironment("Radius.Compute/containers", map[string]any{"containers": "malformed"}))
}
