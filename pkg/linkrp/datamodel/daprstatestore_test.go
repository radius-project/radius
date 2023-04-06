// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	"testing"

	"github.com/project-radius/radius/pkg/linkrp/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/to"
	"github.com/stretchr/testify/require"
)

func TestDaprStateStore_ApplyDeploymentOutput(t *testing.T) {
	tests := []struct {
		name    string
		dss     *DaprStateStore
		do      *rpv1.DeploymentOutput
		wantErr bool
	}{
		{
			name: "with component name",
			dss:  &DaprStateStore{},
			do: &rpv1.DeploymentOutput{
				DeployedOutputResources: []rpv1.OutputResource{
					{
						LocalID: rpv1.LocalIDDaprStateStoreAzureStorage,
						ResourceType: resourcemodel.ResourceType{
							Type:     resourcekinds.DaprStateStoreAzureStorage,
							Provider: resourcemodel.ProviderAzure,
						},
						Identity: resourcemodel.ResourceIdentity{
							ResourceType: &resourcemodel.ResourceType{
								Type:     resourcekinds.DaprStateStoreAzureStorage,
								Provider: resourcemodel.ProviderAzure,
							},
							Data: resourcemodel.ARMIdentity{},
						},
						RadiusManaged: to.Ptr(true),
						Dependencies: []rpv1.Dependency{
							{
								LocalID: "",
							},
						},
					},
				},
				ComputedValues: map[string]any{
					renderers.ComponentNameKey: "dapr-state-store-test",
				},
			},
			wantErr: false,
		},
		{
			name: "without component name",
			dss:  &DaprStateStore{},
			do: &rpv1.DeploymentOutput{
				DeployedOutputResources: []rpv1.OutputResource{
					{
						LocalID: rpv1.LocalIDDaprStateStoreAzureStorage,
						ResourceType: resourcemodel.ResourceType{
							Type:     resourcekinds.DaprStateStoreAzureStorage,
							Provider: resourcemodel.ProviderAzure,
						},
						Identity: resourcemodel.ResourceIdentity{
							ResourceType: &resourcemodel.ResourceType{
								Type:     resourcekinds.DaprStateStoreAzureStorage,
								Provider: resourcemodel.ProviderAzure,
							},
							Data: resourcemodel.ARMIdentity{},
						},
						RadiusManaged: to.Ptr(true),
						Dependencies: []rpv1.Dependency{
							{
								LocalID: "",
							},
						},
					},
				},
				ComputedValues: map[string]any{},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.dss.ApplyDeploymentOutput(*tt.do); (err != nil) != tt.wantErr {
				t.Errorf("DaprStateStore.ApplyDeploymentOutput() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				require.EqualValues(t, tt.do.DeployedOutputResources, tt.dss.Properties.Status.OutputResources)
				require.EqualValues(t, tt.do.ComputedValues, tt.dss.ComputedValues)
				require.EqualValues(t, tt.do.SecretValues, tt.dss.SecretValues)
				require.Condition(t, func() bool {
					if tt.do.ComputedValues[renderers.ComponentNameKey] != nil {
						return tt.dss.Properties.ComponentName == tt.do.ComputedValues[renderers.ComponentNameKey]
					}
					return tt.dss.Properties.ComponentName == ""
				}, "component name should be equal")
			}
		})
	}
}
