/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package reconciler

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/authz"
)

func Test_checkNamespaceMapping(t *testing.T) {
	ctx := t.Context()
	t.Cleanup(func() { authz.SetProcessOptions(authz.Options{}) })

	authz.SetProcessOptions(authz.Options{})
	require.NoError(t, checkNamespaceMapping(ctx, "any", "/planes/radius/local/resourceGroups/prod"))

	authz.SetProcessOptions(authz.Options{Mode: authz.ModeEnforce, NamespaceMappings: []authz.NamespaceMapping{
		{Namespace: "dev", ResourceGroup: "/planes/radius/local/resourceGroups/dev"},
	}})
	require.NoError(t, checkNamespaceMapping(ctx, "dev", "/planes/radius/local/resourceGroups/dev"))

	var denied *authz.DeniedError
	err := checkNamespaceMapping(ctx, "dev", "/planes/radius/local/resourceGroups/prod")
	require.True(t, errors.As(err, &denied))
	require.Equal(t, v1.CodeGrantScopeExceeded, denied.Code)

	err = checkNamespaceMapping(ctx, "other", "/planes/radius/local/resourceGroups/dev")
	require.True(t, errors.As(err, &denied))
	require.Equal(t, v1.CodeAuthorizationFailed, denied.Code)

	authz.SetProcessOptions(authz.Options{Mode: authz.ModeDryRun})
	require.NoError(t, checkNamespaceMapping(ctx, "other", "/planes/radius/local/resourceGroups/dev"))
}
