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
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/stretchr/testify/require"
)

const (
	testEnvID = "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env"
	testAppID = "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/app"
)

func fakeStoreObject(dm conv.DataModelInterface) *store.Object {
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
		prop datamodel.KubernetesComputeProperties
		id   string
		out  string
	}{
		{
			desc: "namespace is defined",
			prop: datamodel.KubernetesComputeProperties{
				Namespace: "default-ns",
			},
			id:  testEnvID,
			out: "default-ns",
		},
		{
			desc: "undefined namespace",
			prop: datamodel.KubernetesComputeProperties{},
			id:   testEnvID,
			out:  "env",
		},
	}

	for _, tc := range nsTests {
		t.Run(tc.desc, func(t *testing.T) {
			envdm := &datamodel.Environment{
				Properties: datamodel.EnvironmentProperties{
					Compute: datamodel.EnvironmentCompute{
						Kind:              datamodel.KubernetesComputeKind,
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

func TestFindNamespaceByAppID(t *testing.T) {
	mctrl := gomock.NewController(t)

	nsTests := []struct {
		desc    string
		envID   string
		envProp datamodel.KubernetesComputeProperties
		appID   string
		appProp *datamodel.KubeNamespaceOverrideExtension
		out     string
	}{
		{
			desc:  "override namespace extension",
			envID: testEnvID,
			envProp: datamodel.KubernetesComputeProperties{
				Namespace: "default-ns",
			},
			appID: testAppID,
			appProp: &datamodel.KubeNamespaceOverrideExtension{
				Namespace: "appoverride",
			},
			out: "appoverride",
		},
		{
			desc:  "concatnate namespace",
			envID: testEnvID,
			envProp: datamodel.KubernetesComputeProperties{
				Namespace: "default-ns",
			},
			appID:   testAppID,
			appProp: nil,
			out:     "default-ns-app",
		},
	}

	for _, tc := range nsTests {
		t.Run(tc.desc, func(t *testing.T) {

			mockSP := dataprovider.NewMockDataStorageProvider(mctrl)
			mockSC := store.NewMockStorageClient(mctrl)

			envdm := &datamodel.Environment{
				Properties: datamodel.EnvironmentProperties{
					Compute: datamodel.EnvironmentCompute{
						Kind:              datamodel.KubernetesComputeKind,
						KubernetesCompute: tc.envProp,
					},
				},
			}

			appdm := &datamodel.Application{
				Properties: datamodel.ApplicationProperties{
					BasicResourceProperties: rp.BasicResourceProperties{
						Environment: tc.envID,
					},
				},
			}

			if tc.appProp != nil {
				appdm.Properties.Extensions = []datamodel.Extension{
					{
						Kind:                        datamodel.KubernetesNamespaceOverride,
						KubernetesNamespaceOverride: tc.appProp,
					},
				}
				mockSP.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Return(store.StorageClient(mockSC), nil).Times(1)
			} else {
				mockSP.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Return(store.StorageClient(mockSC), nil).Times(2)
				mockSC.EXPECT().Get(gomock.Any(), tc.envID, gomock.Any()).Return(fakeStoreObject(envdm), nil).Times(1)
			}

			mockSC.EXPECT().Get(gomock.Any(), tc.appID, gomock.Any()).Return(fakeStoreObject(appdm), nil).Times(1)

			ns, err := FindNamespaceByAppID(context.Background(), mockSP, tc.appID)
			require.NoError(t, err)
			require.Equal(t, tc.out, ns)
		})
	}
}
