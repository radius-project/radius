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

package datamodel

import (
	"testing"

	"github.com/project-radius/radius/pkg/portableresources/renderers"
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
						LocalID:       rpv1.LocalIDDaprStateStoreAzureStorage,
						RadiusManaged: to.Ptr(true),
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
						LocalID:       rpv1.LocalIDDaprStateStoreAzureStorage,
						RadiusManaged: to.Ptr(true),
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
