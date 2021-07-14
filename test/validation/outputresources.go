// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package validation

import (
	"context"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/armcore"
	"github.com/Azure/radius/pkg/radclient"
	"github.com/stretchr/testify/require"
	"gotest.tools/assert"
)

type ComponentSet struct {
	Components []Component
}

type Component struct {
	ComponentName   string
	ApplicationName string
	OutputResources map[string]OutputResourceSet
}

type OutputResourceSet struct {
	LocalID            string
	OutputResourceType string
	ResourceKind       string
	Managed            bool
}

func NewOutputResource(localID, outputResourceType, resourceKind string, managed bool) OutputResourceSet {
	return OutputResourceSet{
		LocalID:            localID,
		OutputResourceType: outputResourceType,
		ResourceKind:       resourceKind,
		Managed:            managed,
	}
}

func ValidateOutputResources(t *testing.T, armConnection *armcore.Connection, subscriptionID string, resourceGroup string, expected ComponentSet) {
	componentsClient := radclient.NewComponentClient(armConnection, subscriptionID)
	for _, c := range expected.Components {
		component, err := componentsClient.Get(context.Background(), resourceGroup, c.ApplicationName, c.ComponentName, nil)
		require.NoError(t, err)
		assert.Equal(t, len(c.OutputResources), len(component.ComponentResource.Properties.OutputResources))
		for _, r := range component.ComponentResource.Properties.OutputResources {
			key := r["localId"].(string)
			assert.Equal(t, c.OutputResources[key].LocalID, r["localId"].(string))
			assert.Equal(t, c.OutputResources[key].OutputResourceType, r["outputResourceType"].(string))
			assert.Equal(t, c.OutputResources[key].ResourceKind, r["resourceKind"].(string))
		}
	}
}
