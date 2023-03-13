// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kube

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/golang/mock/gomock"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	model "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/to"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/store"
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

	ns, err := FetchNameSpaceFromEnvironmentResource(&envResource)
	require.NoError(t, err)
	require.Equal(t, namespace, ns)
	// Invalid env model
	envResource.Properties.Compute = &model.EnvironmentCompute{}
	_, err = FetchNameSpaceFromEnvironmentResource(&envResource)
	require.Error(t, err)
	require.Equal(t, err.Error(), "invalid model conversion")
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

	ns, err := FetchNameSpaceFromApplicationResource(&appResource)
	require.NoError(t, err)
	require.Equal(t, appNamespace, ns)
	// Invalid app model
	appResource.Properties.Status.Compute = &model.EnvironmentCompute{}
	_, err = FetchNameSpaceFromApplicationResource(&appResource)
	require.Error(t, err)
	require.Equal(t, err.Error(), "invalid model conversion")
}
