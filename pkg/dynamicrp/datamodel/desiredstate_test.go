/*
Copyright 2026 The Radius Authors.

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

package datamodel_test

import (
	"encoding/json"
	"testing"

	"github.com/radius-project/radius/pkg/crypto/encryption"
	"github.com/radius-project/radius/pkg/dynamicrp/api"
	"github.com/radius-project/radius/pkg/dynamicrp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestDesiredStateProtection(t *testing.T) {
	t.Parallel()
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	handler, err := encryption.NewSensitiveDataHandlerFromKey(key)
	require.NoError(t, err)
	source := map[string]any{"type": "object", "properties": map[string]any{
		"image":    map[string]any{"type": "string"},
		"password": map[string]any{"type": "string", "x-radius-sensitive": true},
		"endpoint": map[string]any{"type": "string", "readOnly": true},
	}}
	incoming := recipes.IncomingReference{
		ID:   "/planes/radius/local/resourceGroups/rg/providers/Radius.Compute/containers/cache",
		Type: "Radius.Compute/containers", APIVersion: "2025-08-01-preview", Revision: "desired-1",
		Properties: map[string]any{"image": "redis", "password": "private-password"},
	}
	snapshot, err := datamodel.NewDesiredState(t.Context(), incoming, source, handler)
	require.NoError(t, err)
	data, err := json.Marshal(snapshot)
	require.NoError(t, err)
	require.NotContains(t, string(data), "private-password")
	require.NotContains(t, string(data), "redis")

	// A later target-only change to public state must not alter the saved recipe inputs.
	incoming.Properties["image"] = "changed"
	materialized, savedSchema, err := snapshot.Materialize(t.Context(), handler)
	require.NoError(t, err)
	require.Equal(t, "redis", materialized.Properties["image"])
	require.Equal(t, "private-password", materialized.Properties["password"])
	require.Equal(t, source, savedSchema)
	materialized.Properties["password"] = "changed"
	materialized, _, err = snapshot.Materialize(t.Context(), handler)
	require.NoError(t, err)
	require.Equal(t, "private-password", materialized.Properties["password"])

	tampered := *snapshot
	tampered.ResourceID += "-other"
	_, _, err = tampered.Materialize(t.Context(), handler)
	require.Error(t, err)
	tampered = *snapshot
	tampered.Revision = "desired-2"
	_, _, err = tampered.Materialize(t.Context(), handler)
	require.Error(t, err)

	model := &datamodel.DynamicResource{DesiredState: snapshot, DelegationTargetID: incoming.ID, Properties: map[string]any{}}
	model.ID = incoming.ID
	model.Type = incoming.Type
	model.UpdatedAPIVersion = incoming.APIVersion
	keys := encryption.NewMockKeyProvider(gomock.NewController(t))
	keys.EXPECT().GetCurrentKey(gomock.Any()).Return(key, 0, nil)
	restored, err := model.RestoreDesiredInputs(t.Context(), keys)
	require.NoError(t, err)
	require.Equal(t, "private-password", restored["password"])
	require.Equal(t, "redis", restored["image"])
	versioned := &api.DynamicResource{}
	require.NoError(t, versioned.ConvertFrom(model))
	data, err = json.Marshal(versioned)
	require.NoError(t, err)
	require.NotContains(t, string(data), "desiredState")
	require.NotContains(t, string(data), "payload")
	require.NotContains(t, string(data), "delegationTargetId")
}

func TestDesiredStateRejectsComputedInputs(t *testing.T) {
	t.Parallel()
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	handler, err := encryption.NewSensitiveDataHandlerFromKey(key)
	require.NoError(t, err)
	_, err = datamodel.NewDesiredState(t.Context(), recipes.IncomingReference{
		ID:   "/planes/radius/local/resourceGroups/rg/providers/Radius.Compute/routes/route",
		Type: "Radius.Compute/routes", APIVersion: "2025-08-01-preview", Revision: "desired-1",
		Properties: map[string]any{"listener": "computed"},
	}, map[string]any{"type": "object", "properties": map[string]any{
		"listener": map[string]any{"type": "string", "readOnly": true},
	}}, handler)
	require.Error(t, err)
}
