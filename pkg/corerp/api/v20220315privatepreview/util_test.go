// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"testing"
	"time"

	"github.com/project-radius/radius/pkg/api/armrpcv1"
	"github.com/project-radius/radius/pkg/basedatamodel"

	"github.com/stretchr/testify/require"
)

func TestToProvisioningStateDataModel(t *testing.T) {
	stateTests := []struct {
		versioned ProvisioningState
		datamodel basedatamodel.ProvisioningStates
	}{
		{ProvisioningStateUpdating, basedatamodel.ProvisioningStateUpdating},
		{ProvisioningStateDeleting, basedatamodel.ProvisioningStateDeleting},
		{ProvisioningStateAccepted, basedatamodel.ProvisioningStateAccepted},
		{ProvisioningStateSucceeded, basedatamodel.ProvisioningStateSucceeded},
		{ProvisioningStateFailed, basedatamodel.ProvisioningStateFailed},
		{ProvisioningStateCanceled, basedatamodel.ProvisioningStateCanceled},
		{"", basedatamodel.ProvisioningStateAccepted},
	}

	for _, tt := range stateTests {
		sc := toProvisioningStateDataModel(&tt.versioned)
		require.Equal(t, tt.datamodel, sc)
	}
}

func TestFromProvisioningStateDataModel(t *testing.T) {
	stateTests := []struct {
		datamodel basedatamodel.ProvisioningStates
		versioned ProvisioningState
	}{
		{basedatamodel.ProvisioningStateUpdating, ProvisioningStateUpdating},
		{basedatamodel.ProvisioningStateDeleting, ProvisioningStateDeleting},
		{basedatamodel.ProvisioningStateAccepted, ProvisioningStateAccepted},
		{basedatamodel.ProvisioningStateSucceeded, ProvisioningStateSucceeded},
		{basedatamodel.ProvisioningStateFailed, ProvisioningStateFailed},
		{basedatamodel.ProvisioningStateCanceled, ProvisioningStateCanceled},
		{"", ProvisioningStateAccepted},
	}

	for _, tt := range stateTests {
		sc := fromProvisioningStateDataModel(tt.datamodel)
		require.Equal(t, tt.versioned, *sc)
	}
}

func TestUnmarshalTimeString(t *testing.T) {
	parsedTime := unmarshalTimeString("2021-09-24T19:09:00.000000Z")
	require.NotNil(t, parsedTime)

	require.Equal(t, 2021, parsedTime.Year())
	require.Equal(t, time.Month(9), parsedTime.Month())
	require.Equal(t, 24, parsedTime.Day())

	parsedTime = unmarshalTimeString("")
	require.NotNil(t, parsedTime)
	require.Equal(t, 1, parsedTime.Year())
}

func TestFromSystemDataModel(t *testing.T) {
	systemDataTests := []armrpcv1.SystemData{
		{
			CreatedBy:          "",
			CreatedByType:      "",
			CreatedAt:          "",
			LastModifiedBy:     "",
			LastModifiedByType: "",
			LastModifiedAt:     "",
		}, {
			CreatedBy:          "fakeid@live.com",
			CreatedByType:      "",
			CreatedAt:          "2021-09-24T19:09:00Z",
			LastModifiedBy:     "fakeid@live.com",
			LastModifiedByType: "",
			LastModifiedAt:     "2021-09-25T19:09:00Z",
		}, {
			CreatedBy:          "fakeid@live.com",
			CreatedByType:      "User",
			CreatedAt:          "2021-09-24T19:09:00Z",
			LastModifiedBy:     "fakeid@live.com",
			LastModifiedByType: "User",
			LastModifiedAt:     "2021-09-25T19:09:00Z",
		},
	}

	for _, tt := range systemDataTests {
		versioned := fromSystemDataModel(tt)
		require.Equal(t, tt.CreatedBy, string(*versioned.CreatedBy))
		require.Equal(t, tt.CreatedByType, string(*versioned.CreatedByType))
		c, _ := versioned.CreatedAt.MarshalText()
		if tt.CreatedAt == "" {
			tt.CreatedAt = "0001-01-01T00:00:00Z"
		}
		require.Equal(t, tt.CreatedAt, string(c))

		require.Equal(t, tt.LastModifiedBy, string(*versioned.LastModifiedBy))
		require.Equal(t, tt.LastModifiedByType, string(*versioned.LastModifiedByType))
		c, _ = versioned.LastModifiedAt.MarshalText()
		if tt.LastModifiedAt == "" {
			tt.LastModifiedAt = "0001-01-01T00:00:00Z"
		}
		require.Equal(t, tt.LastModifiedAt, string(c))
	}
}
