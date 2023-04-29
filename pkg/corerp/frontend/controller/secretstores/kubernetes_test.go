// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package secretstores

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/test/testutil"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	testRootScope = "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers"
	testSecretID  = testRootScope + "/Applications.Core/secretStores/secret0"
	testEnvID     = testRootScope + "/Applications.Core/environments/env0"
	testAppID     = testRootScope + "/Applications.Core/applications/app0"
)

func TestGetNamespace(t *testing.T) {
	ctrl := gomock.NewController(t)
	sc := store.NewMockStorageClient(ctrl)

	opt := &controller.Options{
		StorageClient: sc,
	}

	t.Run("application-scoped", func(t *testing.T) {
		secret := testutil.MustGetTestData[datamodel.SecretStore]("secretstores_datamodel.json")
		secret.Properties.Application = testAppID
		appData := testutil.MustGetTestData[any]("app_datamodel.json")

		sc.EXPECT().Get(gomock.Any(), testAppID, gomock.Any()).Return(&store.Object{
			Data: *appData,
		}, nil)

		secret.Properties.Application = testAppID
		ns, err := getNamespace(context.TODO(), secret, opt)
		require.NoError(t, err)
		require.Equal(t, "app0-ns", ns)
	})

	t.Run("environment-scoped", func(t *testing.T) {
		secret := testutil.MustGetTestData[datamodel.SecretStore]("secretstores_datamodel.json")
		secret.Properties.Application = ""
		secret.Properties.Environment = testEnvID

		envData := testutil.MustGetTestData[any]("env_datamodel.json")

		sc.EXPECT().Get(gomock.Any(), testEnvID, gomock.Any()).Return(&store.Object{
			Data: *envData,
		}, nil)

		ns, err := getNamespace(context.TODO(), secret, opt)
		require.NoError(t, err)
		require.Equal(t, "default", ns)
	})

	t.Run("non-kubernetes platform", func(t *testing.T) {
		secret := testutil.MustGetTestData[datamodel.SecretStore]("secretstores_datamodel.json")
		secret.Properties.Application = ""
		secret.Properties.Environment = testEnvID
		envData := testutil.MustGetTestData[any]("env_nonk8s_datamodel.json")

		sc.EXPECT().Get(gomock.Any(), testEnvID, gomock.Any()).Return(&store.Object{
			Data: *envData,
		}, nil)

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
			resourceID: "namespace/namE_2",
			err:        errors.New("'namE_2' is the invalid resource name. This must be at most 63 alphanumeric characters or '-'"),
		},
		{
			resourceID: "namespa_ce/name",
			err:        errors.New("'namespa_ce' is the invalid namespace. This must be at most 63 alphanumeric characters or '-'"),
		},
	}

	for _, tc := range resourceTests {
		t.Run(tc.resourceID, func(t *testing.T) {
			ns, name, err := fromResourceID(tc.resourceID)
			if tc.err != nil {
				require.ErrorContains(t, err, tc.err.Error())
			} else {
				require.Equal(t, tc.ns, ns)
				require.Equal(t, tc.name, name)
			}
		})
	}
}

func TestValidateRequest(t *testing.T) {
	t.Run("type is not certificate", func(t *testing.T) {
		newResource := testutil.MustGetTestData[datamodel.SecretStore]("secretstores_datamodel.json")
		newResource.Properties.Type = datamodel.SecretTypeGeneric
		resp, err := ValidateRequest(context.TODO(), newResource, nil, nil)
		require.NoError(t, err)

		// assert
		r := resp.(*rest.BadRequestResponse)
		require.Equal(t, "secret store type generic is not supported.", r.Body.Error.Message)
	})

	t.Run("new resource, but referencing valueFrom", func(t *testing.T) {
		newResource := testutil.MustGetTestData[datamodel.SecretStore]("secretstores_datamodel.json")
		newResource.Properties.Resource = ""
		resp, err := ValidateRequest(context.TODO(), newResource, nil, nil)
		require.NoError(t, err)

		// assert
		r := resp.(*rest.BadRequestResponse)
		require.True(t, r.Body.Error.Message == "data[tls.crt] must not set valueFrom." || r.Body.Error.Message == "data[tls.key] must not set valueFrom.")
	})

	t.Run("update the existing resource - type not matched", func(t *testing.T) {
		oldResource := testutil.MustGetTestData[datamodel.SecretStore]("secretstores_datamodel.json")
		oldResource.Properties.Type = datamodel.SecretTypeGeneric
		newResource := testutil.MustGetTestData[datamodel.SecretStore]("secretstores_datamodel.json")
		resp, err := ValidateRequest(context.TODO(), newResource, oldResource, nil)
		require.NoError(t, err)

		// assert
		r := resp.(*rest.BadRequestResponse)
		require.Equal(t, "type cannot be changed.", r.Body.Error.Message)
	})

	t.Run("resourceID is not same", func(t *testing.T) {
		oldResource := testutil.MustGetTestData[datamodel.SecretStore]("secretstores_datamodel.json")
		oldResource.Properties.Resource = "default/notmatch"
		newResource := testutil.MustGetTestData[datamodel.SecretStore]("secretstores_datamodel.json")
		resp, err := ValidateRequest(context.TODO(), newResource, oldResource, nil)
		require.NoError(t, err)

		// assert
		r := resp.(*rest.BadRequestResponse)
		require.Equal(t, "'default/letencrypt-prod' of $.properties.resource must correspond to 'default/notmatch'.", r.Body.Error.Message)
	})

	t.Run("inherit resource id from existing resource", func(t *testing.T) {
		oldResource := testutil.MustGetTestData[datamodel.SecretStore]("secretstores_datamodel.json")
		newResource := testutil.MustGetTestData[datamodel.SecretStore]("secretstores_datamodel.json")
		newResource.Properties.Resource = ""
		resp, err := ValidateRequest(context.TODO(), newResource, oldResource, nil)

		// assert
		require.NoError(t, err)
		require.Nil(t, resp)
		require.Equal(t, oldResource.Properties.Resource, newResource.Properties.Resource)
	})
}

func TestUpsertSecret(t *testing.T) {
	ctrl := gomock.NewController(t)
	sc := store.NewMockStorageClient(ctrl)

	appData := testutil.MustGetTestData[any]("app_datamodel.json")

	sc.EXPECT().Get(gomock.Any(), testAppID, gomock.Any()).Return(&store.Object{
		Data: *appData,
	}, nil)

	t.Run("create new secret with the specified resource", func(t *testing.T) {
		newResource := testutil.MustGetTestData[datamodel.SecretStore]("secretstores_datamodel_value.json")
		newResource.Properties.Resource = "default/secret"

		opt := &controller.Options{
			KubeClient: testutil.NewFakeKubeClient(nil),
		}

		resp, err := UpsertSecret(context.TODO(), newResource, nil, opt)
		require.NoError(t, err)
		require.Nil(t, resp)

		// assert
		ksecret := &corev1.Secret{}
		err = opt.KubeClient.Get(context.TODO(), runtimeclient.ObjectKey{Namespace: "default", Name: "secret"}, ksecret)
		require.NoError(t, err)
		require.Equal(t, "dGxzLmtleS1wcmlrZXkK", string(ksecret.Data["tls.crt"]))
		require.Equal(t, "dGxzLmNlcnQK", string(ksecret.Data["tls.key"]))
		require.Equal(t, rpv1.OutputResource{
			Identity: resourcemodel.ResourceIdentity{
				ResourceType: &resourcemodel.ResourceType{
					Type:     resourcekinds.Secret,
					Provider: resourcemodel.ProviderKubernetes,
				},
				Data: resourcemodel.KubernetesIdentity{
					Kind:       resourcekinds.Secret,
					APIVersion: "v1",
					Name:       "secret",
					Namespace:  "default",
				},
			},
		}, newResource.Properties.Status.OutputResources[0])
	})

	t.Run("not found referenced key", func(t *testing.T) {
		newResource := testutil.MustGetTestData[datamodel.SecretStore]("secretstores_datamodel.json")

		ksecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "letencrypt-prod",
				Namespace: "default",
			},
			Data: map[string][]byte{},
		}
		opt := &controller.Options{
			KubeClient: testutil.NewFakeKubeClient(nil, ksecret),
		}

		resp, err := UpsertSecret(context.TODO(), newResource, nil, opt)
		require.NoError(t, err)

		// assert
		r := resp.(*rest.BadRequestResponse)
		require.Equal(t, "default/letencrypt-prod does not have key, tls.crt.", r.Body.Error.Message)
	})

	t.Run("add secret values to the existing secret store", func(t *testing.T) {
		newResource := testutil.MustGetTestData[datamodel.SecretStore]("secretstores_datamodel_value.json")
		newResource.Properties.Resource = "default/secret"

		ksecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "secret",
				Namespace: "default",
			},
			Data: map[string][]byte{
				"private.key": []byte("private key value"),
			},
		}
		opt := &controller.Options{
			KubeClient: testutil.NewFakeKubeClient(nil, ksecret),
		}

		resp, err := UpsertSecret(context.TODO(), newResource, nil, opt)
		require.NoError(t, err)
		require.Nil(t, resp)

		// assert
		actual := &corev1.Secret{}
		err = opt.KubeClient.Get(context.TODO(), runtimeclient.ObjectKey{Namespace: "default", Name: "secret"}, actual)
		require.NoError(t, err)
		require.Equal(t, "dGxzLmtleS1wcmlrZXkK", string(actual.Data["tls.crt"]))
		require.Equal(t, "dGxzLmNlcnQK", string(actual.Data["tls.key"]))
		require.Equal(t, "private key value", string(actual.Data["private.key"]))
	})

	t.Run("disallow to use secret key name unmatched with valueFrom", func(t *testing.T) {
		newResource := testutil.MustGetTestData[datamodel.SecretStore]("secretstores_datamodel.json")
		newResource.Properties.Data["diffkey"] = &datamodel.SecretStoreDataValue{
			ValueFrom: &datamodel.SecretStoreDataValueFrom{Name: "key"},
		}

		ksecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "letencrypt-prod",
				Namespace: "default",
			},
			Data: map[string][]byte{
				"tls.key": []byte("key"),
				"tls.crt": []byte("cert"),
			},
		}
		opt := &controller.Options{
			KubeClient: testutil.NewFakeKubeClient(nil, ksecret),
		}

		resp, err := UpsertSecret(context.TODO(), newResource, nil, opt)
		require.NoError(t, err)

		// assert
		r := resp.(*rest.BadRequestResponse)
		require.Equal(t, "diffkey key name must be same as valueFrom.name key.", r.Body.Error.Message)
	})

	t.Run("inherit old resource id", func(t *testing.T) {
		oldResource := testutil.MustGetTestData[datamodel.SecretStore]("secretstores_datamodel.json")
		newResource := testutil.MustGetTestData[datamodel.SecretStore]("secretstores_datamodel.json")
		newResource.Properties.Resource = ""

		opt := &controller.Options{
			KubeClient: testutil.NewFakeKubeClient(nil),
		}

		_, err := UpsertSecret(context.TODO(), newResource, oldResource, opt)
		require.NoError(t, err)

		// assert
		require.Equal(t, oldResource.Properties.Resource, newResource.Properties.Resource)
	})

	t.Run("create new resource", func(t *testing.T) {
		newResource := testutil.MustGetTestData[datamodel.SecretStore]("secretstores_datamodel.json")
		newResource.Properties.Resource = ""

		opt := &controller.Options{
			StorageClient: sc,
			KubeClient:    testutil.NewFakeKubeClient(nil),
		}

		_, err := UpsertSecret(context.TODO(), newResource, nil, opt)
		require.NoError(t, err)

		// assert
		require.Equal(t, "app0-ns/secret0", newResource.Properties.Resource)
	})
}

func TestDeleteSecret(t *testing.T) {
	t.Run("delete secret created by Radius", func(t *testing.T) {
		res := testutil.MustGetTestData[datamodel.SecretStore]("secretstores_datamodel.json")

		ksecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "letencrypt-prod",
				Namespace: "default",
				Labels: map[string]string{
					kubernetes.LabelRadiusResourceType: "test",
				},
			},
			Data: map[string][]byte{},
		}
		opt := &controller.Options{
			KubeClient: testutil.NewFakeKubeClient(nil, ksecret),
		}

		resp, err := DeleteRadiusSecret(context.TODO(), res, opt)
		require.NoError(t, err)
		require.Nil(t, resp)

		err = opt.KubeClient.Get(context.TODO(), runtimeclient.ObjectKey{Namespace: "default", Name: "letencrypt-prod"}, ksecret)
		require.True(t, apierrors.IsNotFound(err))
	})

	t.Run("not delete secret unless secret resource has radius label", func(t *testing.T) {
		res := testutil.MustGetTestData[datamodel.SecretStore]("secretstores_datamodel.json")

		ksecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "letencrypt-prod",
				Namespace: "default",
			},
			Data: map[string][]byte{},
		}
		opt := &controller.Options{
			KubeClient: testutil.NewFakeKubeClient(nil, ksecret),
		}

		resp, err := DeleteRadiusSecret(context.TODO(), res, opt)
		require.NoError(t, err)
		require.Nil(t, resp)

		err = opt.KubeClient.Get(context.TODO(), runtimeclient.ObjectKey{Namespace: "default", Name: "letencrypt-prod"}, ksecret)
		require.False(t, apierrors.IsNotFound(err))
	})

}
