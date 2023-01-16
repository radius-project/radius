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
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/stretchr/testify/require"
)

const (
	testEnvID = "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env"
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
		prop rp.KubernetesComputeProperties
		id   string
		out  string
	}{
		{
			desc: "namespace is defined",
			prop: rp.KubernetesComputeProperties{
				Namespace: "default-ns",
			},
			id:  testEnvID,
			out: "default-ns",
		},
		{
			desc: "undefined namespace",
			prop: rp.KubernetesComputeProperties{},
			id:   testEnvID,
			out:  "env",
		},
	}

	for _, tc := range nsTests {
		t.Run(tc.desc, func(t *testing.T) {
			envdm := &datamodel.Environment{
				Properties: datamodel.EnvironmentProperties{
					Compute: rp.EnvironmentCompute{
						Kind:              rp.KubernetesComputeKind,
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
