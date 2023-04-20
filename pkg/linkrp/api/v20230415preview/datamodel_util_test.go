// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20230415preview

import (
	"testing"
	"time"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"

	"github.com/stretchr/testify/require"
)

func TestToProvisioningStateDataModel(t *testing.T) {
	stateTests := []struct {
		versioned ProvisioningState
		datamodel v1.ProvisioningState
	}{
		{ProvisioningStateUpdating, v1.ProvisioningStateUpdating},
		{ProvisioningStateDeleting, v1.ProvisioningStateDeleting},
		{ProvisioningStateAccepted, v1.ProvisioningStateAccepted},
		{ProvisioningStateSucceeded, v1.ProvisioningStateSucceeded},
		{ProvisioningStateFailed, v1.ProvisioningStateFailed},
		{ProvisioningStateCanceled, v1.ProvisioningStateCanceled},
		{"", v1.ProvisioningStateAccepted},
	}

	for _, tt := range stateTests {
		sc := toProvisioningStateDataModel(&tt.versioned)
		require.Equal(t, tt.datamodel, sc)
	}
}

func TestFromProvisioningStateDataModel(t *testing.T) {
	testCases := []struct {
		datamodel v1.ProvisioningState
		versioned ProvisioningState
	}{
		{v1.ProvisioningStateUpdating, ProvisioningStateUpdating},
		{v1.ProvisioningStateDeleting, ProvisioningStateDeleting},
		{v1.ProvisioningStateAccepted, ProvisioningStateAccepted},
		{v1.ProvisioningStateSucceeded, ProvisioningStateSucceeded},
		{v1.ProvisioningStateFailed, ProvisioningStateFailed},
		{v1.ProvisioningStateCanceled, ProvisioningStateCanceled},
		{"", ProvisioningStateAccepted},
	}

	for _, testCase := range testCases {
		sc := fromProvisioningStateDataModel(testCase.datamodel)
		require.Equal(t, testCase.versioned, *sc)
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
	systemDataTests := []v1.SystemData{
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
