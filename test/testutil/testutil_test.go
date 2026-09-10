package testutil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTestImageReferences(test *testing.T) {
	testCases := []struct {
		name             string
		registry         string
		tag              string
		expectedRegistry string
		expectedTag      string
	}{
		{
			name:             "local default does not follow a release alias",
			expectedRegistry: "ghcr.io/radius-project",
			expectedTag:      "test-local",
		},
		{
			name:             "test run identity is preserved",
			registry:         "localhost:5000",
			tag:              "test-functional-123-2",
			expectedRegistry: "localhost:5000",
			expectedTag:      "test-functional-123-2",
		},
	}
	for _, testCase := range testCases {
		test.Run(testCase.name, func(test *testing.T) {
			test.Setenv("DOCKER_REGISTRY", testCase.registry)
			test.Setenv("REL_VERSION", testCase.tag)
			registry, tag := SetDefault()
			require.Equal(test, testCase.expectedRegistry, registry)
			require.Equal(test, testCase.expectedTag, tag)
			require.Equal(test, "magpieimage="+registry+"/magpiego:"+tag, GetMagpieImage())
			require.Equal(test, "magpietag="+tag, GetMagpieTag())
		})
	}
}
