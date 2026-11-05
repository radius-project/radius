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

package cmd

import (
	"testing"

	"github.com/radius-project/radius/pkg/cli/workspaces"
	v20250801preview "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/stretchr/testify/require"
)

func TestPopulateRecipePackClients(t *testing.T) {
	scope := "/planes/radius/local/resourceGroups/test-group"

	t.Run("no-op for empty packIDs", func(t *testing.T) {
		clientsByScope := map[string]*v20250801preview.RecipePacksClient{
			scope: {}, // placeholder client
		}

		err := PopulateRecipePackClients(t.Context(), &workspaces.Workspace{Scope: scope}, clientsByScope, nil)
		require.NoError(t, err)
		require.Len(t, clientsByScope, 1)
	})

	t.Run("skips packs whose scope is already in the map", func(t *testing.T) {
		clientsByScope := map[string]*v20250801preview.RecipePacksClient{
			scope: {},
		}
		packIDs := []string{
			scope + "/providers/Radius.Core/recipePacks/pack1",
			scope + "/providers/Radius.Core/recipePacks/pack2",
		}

		err := PopulateRecipePackClients(t.Context(), &workspaces.Workspace{Scope: scope}, clientsByScope, packIDs)
		require.NoError(t, err)
		require.Len(t, clientsByScope, 1, "no new scopes should be added")
	})
}
