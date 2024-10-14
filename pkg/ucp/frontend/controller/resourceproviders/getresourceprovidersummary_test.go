package resourceproviders

import (
	"errors"
	"testing"

	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/require"
)

func TestExtractScopeAndName(t *testing.T) {
	tests := []struct {
		name          string
		relativePath  string
		expectedScope resources.ID
		expectedName  string
		expectedErr   error
	}{
		{
			name:          "Valid path with trailing slash",
			relativePath:  "/planes/radius/local/providers/Applications.Test/",
			expectedScope: resources.MustParse("/planes/radius/local"),
			expectedName:  "Applications.Test",
			expectedErr:   nil,
		},
		{
			name:          "Valid path without trailing slash",
			relativePath:  "/planes/radius/local/providers/Applications.Test",
			expectedScope: resources.MustParse("/planes/radius/local"),
			expectedName:  "Applications.Test",
			expectedErr:   nil,
		},
		{
			name:          "Invalid path with no separator",
			relativePath:  "planesradiuslocalprovidersApplicationsTest",
			expectedScope: resources.ID{},
			expectedName:  "",
			expectedErr:   errors.New("invalid URL path"),
		},
		{
			name:          "Invalid scope",
			relativePath:  "/planes/radius/local/providers/Applications.Test/some/other/path",
			expectedScope: resources.ID{},
			expectedName:  "",
			expectedErr:   errors.New("\"/planes/radius/local/providers/Applications.Test/some/other\" is a valid resource id but does not refer to a scope"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &GetResourceProviderSummary{}
			scope, name, err := r.extractScopeAndName(tt.relativePath)

			if tt.expectedErr != nil {
				require.Error(t, err, "expected error %v, got nil", tt.expectedErr)
				require.Equal(t, tt.expectedErr.Error(), err.Error(), "expected error %v, got %v", tt.expectedErr, err)
			} else {
				require.NoError(t, err, "expected no error, got %v", err)
			}

			require.Equal(t, tt.expectedScope, scope, "expected scope %v, got %v", tt.expectedScope, scope)
			require.Equal(t, tt.expectedName, name, "expected name %v, got %v", tt.expectedName, name)
		})
	}
}
