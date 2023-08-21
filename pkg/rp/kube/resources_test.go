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

	"github.com/golang/mock/gomock"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	model "github.com/radius-project/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/dataprovider"
	"github.com/radius-project/radius/pkg/ucp/store"
	"github.com/stretchr/testify/require"
)

const (
	testEnvID    = "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env"
	namespace    = "default"
	appNamespace = "app-default"
)

func fakeStoreObject(dm v1.DataModelInterface) *store.Object {
	b, err := json.Marshal(dm)
	if err != nil {
		return nil
	}
	var r any
	err = json.Unmarshal(b, &r)
	if err != nil {
		return nil
	}
	return &store.Object{Data: r}
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

			mockSP := dataprovider.NewMockDataStorageProvider(mctrl)
			mockSC := store.NewMockStorageClient(mctrl)

			mockSP.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Return(store.StorageClient(mockSC), nil).Times(1)
			mockSC.EXPECT().Get(gomock.Any(), tc.id, gomock.Any()).Return(fakeStoreObject(envdm), nil).Times(1)
			ns, err := FindNamespaceByEnvID(context.Background(), mockSP, testEnvID)
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
