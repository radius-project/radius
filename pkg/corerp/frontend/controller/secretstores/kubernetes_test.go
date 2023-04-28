// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package secretstores

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/test/testutil"
	"github.com/stretchr/testify/require"
)

const (
	testSecretID = "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Applications.Core/secretStores/secret0"
	testEnvID    = "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Applications.Core/environments/env0"
	testAppID    = "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Applications.Core/applications/app0"
)

func TestGetNamespace(t *testing.T) {
	ctrl := gomock.NewController(t)
	sc := store.NewMockStorageClient(ctrl)

	secret := &datamodel.SecretStore{}
	err := json.Unmarshal(testutil.ReadFixture("secretstores_datamodel.json"), secret)
	require.NoError(t, err)

	opt := &controller.Options{
		StorageClient: sc,
	}

	t.Run("application-scoped", func(t *testing.T) {
		var appData any
		err = json.Unmarshal(testutil.ReadFixture("app_datamodel.json"), &appData)
		require.NoError(t, err)

		sc.EXPECT().Get(gomock.Any(), testAppID, gomock.Any()).Return(&store.Object{
			Data: appData,
		}, nil).AnyTimes()

		secret.Properties.Application = testAppID
		ns, err := getNamespace(context.TODO(), secret, opt)
		require.NoError(t, err)
		require.Equal(t, "app0-ns", ns)
	})

	t.Run("environment-scoped", func(t *testing.T) {
		secret.Properties.Application = ""
		secret.Properties.Environment = testEnvID

		var envData any
		err = json.Unmarshal(testutil.ReadFixture("env_datamodel.json"), &envData)
		require.NoError(t, err)

		sc.EXPECT().Get(gomock.Any(), testEnvID, gomock.Any()).Return(&store.Object{
			Data: envData,
		}, nil)

		ns, err := getNamespace(context.TODO(), secret, opt)
		require.NoError(t, err)
		require.Equal(t, "default", ns)
	})

	t.Run("non-kubernetes platform", func(t *testing.T) {
		var envData any
		err = json.Unmarshal(testutil.ReadFixture("env_nonk8s_datamodel.json"), &envData)
		require.NoError(t, err)

		sc.EXPECT().Get(gomock.Any(), testEnvID, gomock.Any()).Return(&store.Object{
			Data: envData,
		}, nil)

		secret.Properties.Application = ""
		secret.Properties.Environment = testEnvID
		_, err := getNamespace(context.TODO(), secret, opt)
		require.Error(t, err)
	})
}

func TestToResourceID(t *testing.T) {
	require.Equal(t, "namespace/name", toResourceID("namespace", "name"))
}

func TestFromResourceID(t *testing.T) {
	resourceTests := []struct {
		resourceID string
		ns         string
		name       string
		err        error
	}{
		{
			resourceID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/httproutes/hrt0",
			err:        errors.New("'/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/httproutes/hrt0' is the invalid resource id"),
		},
		{
			resourceID: "name",
			ns:         "",
			name:       "name",
			err:        nil,
		},
		{
			resourceID: "namespace/name",
			ns:         "namespace",
			name:       "name",
			err:        nil,
		},
		{
			resourceID: "",
			err:        errors.New("'' is the invalid resource name. This must be at most 63 alphanumeric characters or '-'"),
		},
		{
			resourceID: "namespace/namE_2",
			err:        errors.New("'namE_2' is the invalid resource name. This must be at most 63 alphanumeric characters or '-'"),
		},
		{
			resourceID: "namespa_ce/name",
			err:        errors.New("'namespa_ce' is the invalid namespace. This must be at most 63 alphanumeric characters or '-'"),
		},
	}

	for _, tc := range resourceTests {
		ns, name, err := fromResourceID(tc.resourceID)
		if err != nil {
			require.ErrorContains(t, err, tc.err.Error())
		} else {
			require.Equal(t, tc.ns, ns)
			require.Equal(t, tc.name, name)
		}
	}
}

func TestValidateRequest(t *testing.T) {
	newResource := &datamodel.SecretStore{}
	err := json.Unmarshal(testutil.ReadFixture("secretstores_datamodel.json"), newResource)
	require.NoError(t, err)

	oldResource := &datamodel.SecretStore{}
	err = json.Unmarshal(testutil.ReadFixture("secretstores_datamodel.json"), oldResource)
	require.NoError(t, err)

	t.Run("new resource", func(t *testing.T) {
		resp, err := ValidateRequest(context.TODO(), newResource, nil, nil)
		require.NoError(t, err)
		require.Nil(t, resp)
	})

	t.Run("update the existing resource", func(t *testing.T) {

	})

	t.Run("type is not certificate", func(t *testing.T) {

	})

	t.Run("resourceID is not same", func(t *testing.T) {

	})
}

func TestUpsertSecret_CreateNewSecret(t *testing.T) {
	t.Run("no existing secretstore resource", func(t *testing.T) {

	})

	t.Run("existing secretstore resource", func(t *testing.T) {
	})

	t.Run("unsupported types", func(t *testing.T) {

	})
}

func TestUpsertSecret_ReuseSecret(t *testing.T) {

}

func TestDeleteSecret(t *testing.T) {

}
