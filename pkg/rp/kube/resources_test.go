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

package kube

import (
	"context"
	"encoding/json"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/components/database"
	model "github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/to"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

const (
	testEnvID         = "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env"
	testEnvIDV2       = "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Radius.Core/environments/env"
	namespace         = "default"
	appNamespace      = "app-default"
	customNamespace   = "custom-ns"
)

func fakeStoreObject(dm v1.DataModelInterface) *database.Object {
	b, err := json.Marshal(dm)
	if err != nil {
		return nil
	}
	var r any
	err = json.Unmarshal(b, &r)
	if err != nil {
		return nil
	}
	return &database.Object{Data: r}
}

func TestFindNamespaceByEnvID(t *testing.T) {
	mctrl := gomock.NewController(t)

	nsTests := []struct {
		desc string
		prop rpv1.KubernetesComputeProperties
		id   string
		out  string
	}{
		{
			desc: "namespace is defined",
			prop: rpv1.KubernetesComputeProperties{
				Namespace: "default-ns",
			},
			id:  testEnvID,
			out: "default-ns",
		},
		{
			desc: "undefined namespace",
			prop: rpv1.KubernetesComputeProperties{},
			id:   testEnvID,
			out:  "env",
		},
	}

	for _, tc := range nsTests {
		t.Run(tc.desc, func(t *testing.T) {
			envdm := &datamodel.Environment{
				Properties: datamodel.EnvironmentProperties{
					Compute: rpv1.EnvironmentCompute{
						Kind:              rpv1.KubernetesComputeKind,
						KubernetesCompute: tc.prop,
					},
				},
			}

			mockSC := database.NewMockClient(mctrl)
			mockSC.EXPECT().Get(gomock.Any(), tc.id, gomock.Any()).Return(fakeStoreObject(envdm), nil).Times(1)

			ns, err := FindNamespaceByEnvID(context.Background(), mockSC, testEnvID)
			require.NoError(t, err)
			require.Equal(t, tc.out, ns)
		})
	}
}

func TestFetchNameSpaceFromEnvironmentResource(t *testing.T) {
	envResource := model.EnvironmentResource{
		Properties: &model.EnvironmentProperties{
			Compute: &model.KubernetesCompute{
				Namespace: to.Ptr(namespace),
			},
		},
	}

	ns, err := FetchNamespaceFromEnvironmentResource(&envResource)
	require.NoError(t, err)
	require.Equal(t, namespace, ns)
	// Invalid env model
	envResource.Properties.Compute = &model.EnvironmentCompute{}
	_, err = FetchNamespaceFromEnvironmentResource(&envResource)
	require.Error(t, err)
	require.Equal(t, err.Error(), "invalid model conversion")
	// Invalid compute fields
	envResource.Properties.Compute = nil
	_, err = FetchNamespaceFromEnvironmentResource(&envResource)
	require.Error(t, err)
	require.Equal(t, err.Error(), "unable to fetch namespace information")
}

func TestFetchNameSpaceFromApplicationResource(t *testing.T) {
	appResource := model.ApplicationResource{
		Properties: &model.ApplicationProperties{
			Status: &model.ResourceStatus{
				Compute: &model.KubernetesCompute{
					Namespace: to.Ptr(appNamespace),
				},
			},
		},
	}

	ns, err := FetchNamespaceFromApplicationResource(&appResource)
	require.NoError(t, err)
	require.Equal(t, appNamespace, ns)
	// Invalid app model
	appResource.Properties.Status.Compute = &model.EnvironmentCompute{}
	_, err = FetchNamespaceFromApplicationResource(&appResource)
	require.Error(t, err)
	require.Equal(t, err.Error(), "invalid model conversion")
	// Invalid compute fields
	appResource.Properties.Status.Compute = nil
	_, err = FetchNamespaceFromApplicationResource(&appResource)
	require.Error(t, err)
	require.Equal(t, err.Error(), "unable to fetch namespace information")

}

func TestFindNamespaceByEnvID_V20250801Preview(t *testing.T) {
	mctrl := gomock.NewController(t)

	tests := []struct {
		desc                  string
		environment           *datamodel.Environment_v20250801preview
		expectedNamespace     string
		expectedError         string
	}{
		{
			desc: "namespace is defined in kubernetes provider",
			environment: &datamodel.Environment_v20250801preview{
				Properties: datamodel.EnvironmentProperties_v20250801preview{
					Providers: &datamodel.Providers_v20250801preview{
						Kubernetes: &datamodel.ProvidersKubernetes_v20250801preview{
							Namespace: customNamespace,
						},
					},
				},
			},
			expectedNamespace: customNamespace,
		},
		{
			desc: "kubernetes provider is nil - defaults to environment name",
			environment: &datamodel.Environment_v20250801preview{
				Properties: datamodel.EnvironmentProperties_v20250801preview{
					Providers: nil,
				},
			},
			expectedNamespace: "env",
		},
		{
			desc: "providers is nil - defaults to environment name",
			environment: &datamodel.Environment_v20250801preview{
				Properties: datamodel.EnvironmentProperties_v20250801preview{
					Providers: nil,
				},
			},
			expectedNamespace: "env",
		},
		{
			desc: "kubernetes provider exists but namespace is empty - uses empty namespace",
			environment: &datamodel.Environment_v20250801preview{
				Properties: datamodel.EnvironmentProperties_v20250801preview{
					Providers: &datamodel.Providers_v20250801preview{
						Kubernetes: &datamodel.ProvidersKubernetes_v20250801preview{
							Namespace: "",
						},
					},
				},
			},
			expectedNamespace: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			mockSC := database.NewMockClient(mctrl)
			mockSC.EXPECT().Get(gomock.Any(), testEnvIDV2, gomock.Any()).Return(fakeStoreObject(tc.environment), nil).Times(1)

			ns, err := FindNamespaceByEnvID(context.Background(), mockSC, testEnvIDV2)
			if tc.expectedError != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expectedError)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectedNamespace, ns)
			}
		})
	}
}

func TestFindNamespaceByEnvID_InvalidResourceType(t *testing.T) {
	mctrl := gomock.NewController(t)
	mockSC := database.NewMockClient(mctrl)

	invalidEnvID := "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Invalid.Type/environments/env"
	
	_, err := FindNamespaceByEnvID(context.Background(), mockSC, invalidEnvID)
	require.Error(t, err)
	require.Equal(t, "invalid environment resource id - must be Applications.Core/environments or Radius.Core/environments", err.Error())
}

func TestFindNamespaceByEnvID_NonKubernetesEnvironment(t *testing.T) {
	mctrl := gomock.NewController(t)

	envdm := &datamodel.Environment{
		Properties: datamodel.EnvironmentProperties{
			Compute: rpv1.EnvironmentCompute{
				Kind: "NonKubernetes",
			},
		},
	}

	mockSC := database.NewMockClient(mctrl)
	mockSC.EXPECT().Get(gomock.Any(), testEnvID, gomock.Any()).Return(fakeStoreObject(envdm), nil).Times(1)

	_, err := FindNamespaceByEnvID(context.Background(), mockSC, testEnvID)
	require.Error(t, err)
	require.Equal(t, ErrNonKubernetesEnvironment, err)
}

func TestFetchNamespaceFromEnvironmentResourceV20250801(t *testing.T) {
	tests := []struct {
		desc                  string
		environment           *v20250801preview.EnvironmentResource
		expectedNamespace     string
	}{
		{
			desc: "namespace is defined in kubernetes provider",
			environment: &v20250801preview.EnvironmentResource{
				Properties: &v20250801preview.EnvironmentProperties{
					Providers: &v20250801preview.Providers{
						Kubernetes: &v20250801preview.ProvidersKubernetes{
							Namespace: to.Ptr(customNamespace),
						},
					},
				},
			},
			expectedNamespace: customNamespace,
		},
		{
			desc: "kubernetes provider is nil - returns empty string",
			environment: &v20250801preview.EnvironmentResource{
				Properties: &v20250801preview.EnvironmentProperties{
					Providers: &v20250801preview.Providers{
						Kubernetes: nil,
					},
				},
			},
			expectedNamespace: "",
		},
		{
			desc: "providers is nil - returns empty string",
			environment: &v20250801preview.EnvironmentResource{
				Properties: &v20250801preview.EnvironmentProperties{
					Providers: nil,
				},
			},
			expectedNamespace: "",
		},
		{
			desc: "kubernetes provider exists but namespace is nil - returns empty string",
			environment: &v20250801preview.EnvironmentResource{
				Properties: &v20250801preview.EnvironmentProperties{
					Providers: &v20250801preview.Providers{
						Kubernetes: &v20250801preview.ProvidersKubernetes{
							Namespace: nil,
						},
					},
				},
			},
			expectedNamespace: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			ns := FetchNamespaceFromEnvironmentResourceV20250801(tc.environment)
			require.Equal(t, tc.expectedNamespace, ns)
		})
	}
}
