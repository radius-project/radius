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

package secretstores

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/kubernetes"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	resources_kubernetes "github.com/radius-project/radius/pkg/ucp/resources/kubernetes"
	"github.com/radius-project/radius/test/k8sutil"
	"github.com/radius-project/radius/test/testutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
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

	testFileCertValueFrom               = "secretstores_datamodel_cert_valuefrom.json"
	testFileCertValue                   = "secretstores_datamodel_cert_value.json"
	testFileGenericValue                = "secretstores_datamodel_generic.json"
	testFileGenericValueGlobalScope     = "secretstores_datamodel_global_scope.json"
	testFileGenericValueInvalidResource = "secretstores_datamodel_global_scope_invalid_resource.json"
	testFileGenericValueEmptyResource   = "secretstores_datamodel_global_scope_empty_resource.json"

	testFileBasicAuthentication        = "secretstores_datamodel_basicauth.json"
	testFileBasicAuthenticationInvalid = "secretstores_datamodel_basicauth_invalid.json"
	testFileAWSIRSA                    = "secretstores_datamodel_awsirsa.json"
	testFileAzureWorkloadIdentity      = "secretstores_datamodel_azwi.json"
)

func TestGetNamespace(t *testing.T) {
	ctrl := gomock.NewController(t)
	sc := database.NewMockClient(ctrl)

	opt := &controller.Options{
		DatabaseClient: sc,
	}

	t.Run("application-scoped", func(t *testing.T) {
		secret := testutil.MustGetTestData[datamodel.SecretStore](testFileCertValueFrom)
		secret.Properties.Application = testAppID
		appData := testutil.MustGetTestData[any]("app_datamodel.json")

		sc.EXPECT().Get(gomock.Any(), testAppID, gomock.Any()).Return(&database.Object{
			Data: *appData,
		}, nil)

		secret.Properties.Application = testAppID
		ns, err := getNamespace(context.TODO(), secret, opt)
		require.NoError(t, err)
		require.Equal(t, "app0-ns", ns)
	})

	t.Run("environment-scoped", func(t *testing.T) {
		secret := testutil.MustGetTestData[datamodel.SecretStore](testFileCertValueFrom)
		secret.Properties.Application = ""
		secret.Properties.Environment = testEnvID

		envData := testutil.MustGetTestData[any]("env_datamodel.json")

		sc.EXPECT().Get(gomock.Any(), testEnvID, gomock.Any()).Return(&database.Object{
			Data: *envData,
		}, nil)

		ns, err := getNamespace(context.TODO(), secret, opt)
		require.NoError(t, err)
		require.Equal(t, "default", ns)
	})

	t.Run("non-kubernetes platform", func(t *testing.T) {
		secret := testutil.MustGetTestData[datamodel.SecretStore](testFileCertValueFrom)
		secret.Properties.Application = ""
		secret.Properties.Environment = testEnvID
		envData := testutil.MustGetTestData[any]("env_nonk8s_datamodel.json")

		sc.EXPECT().Get(gomock.Any(), testEnvID, gomock.Any()).Return(&database.Object{
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
			resourceID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/containers/c",
			err:        errors.New("'/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/containers/c' is the invalid resource id"),
		},
		{
			resourceID: "name",
			ns:         "",
			name:       "name",
			err:        nil,
		},
		{
			resourceID: "",
			ns:         "",
			name:       "",
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

func TestGetOrDefaultType(t *testing.T) {
	tests := []struct {
		in  datamodel.SecretType
		out datamodel.SecretType
		err error
	}{
		{
			in:  datamodel.SecretTypeNone,
			out: datamodel.SecretTypeGeneric,
			err: nil,
		}, {
			in:  datamodel.SecretTypeCert,
			out: datamodel.SecretTypeCert,
			err: nil,
		}, {
			in:  datamodel.SecretTypeGeneric,
			out: datamodel.SecretTypeGeneric,
			err: nil,
		}, {
			in:  "invalid",
			out: "invalid",
			err: errors.New("'invalid' is invalid secret type"),
		},
	}

	for _, tc := range tests {
		t.Run(string(tc.in), func(t *testing.T) {
			actual, err := getOrDefaultType(tc.in)
			if tc.err != nil {
				require.ErrorContains(t, err, tc.err.Error())
			} else {
				require.Equal(t, tc.out, actual)
			}
		})
	}
}

func TestGetOrDefaultEncoding(t *testing.T) {
	tests := []struct {
		secretType datamodel.SecretType
		inenc      datamodel.SecretValueEncoding
		outenc     datamodel.SecretValueEncoding
		err        error
	}{
		{
			secretType: datamodel.SecretTypeCert,
			inenc:      datamodel.SecretValueEncodingBase64,
			outenc:     datamodel.SecretValueEncodingBase64,
			err:        nil,
		}, {
			secretType: datamodel.SecretTypeCert,
			inenc:      datamodel.SecretValueEncodingRaw,
			err:        errors.New("certificate type doesn't support raw"),
		}, {
			secretType: datamodel.SecretTypeGeneric,
			inenc:      datamodel.SecretValueEncodingRaw,
			outenc:     datamodel.SecretValueEncodingRaw,
			err:        nil,
		}, {
			secretType: datamodel.SecretTypeGeneric,
			inenc:      datamodel.SecretValueEncodingBase64,
			outenc:     datamodel.SecretValueEncodingBase64,
			err:        nil,
		}, {
			secretType: datamodel.SecretTypeGeneric,
			inenc:      "invalid",
			err:        errors.New("invalid is the invalid encoding type"),
		},
	}

	for _, tc := range tests {
		name := fmt.Sprintf("%s - type: %s", tc.inenc, tc.secretType)
		t.Run(name, func(t *testing.T) {
			actual, err := getOrDefaultEncoding(tc.secretType, tc.inenc)
			if tc.err != nil {
				require.ErrorContains(t, err, tc.err.Error())
			} else {
				require.Equal(t, tc.outenc, actual)
			}
		})
	}
}

func TestValidateAndMutateRequest(t *testing.T) {
	tests := []struct {
		name           string
		testFile       string
		oldResource    *datamodel.SecretStore
		modifyResource func(*datamodel.SecretStore, *datamodel.SecretStore)
		assertions     func(*testing.T, rest.Response, error, *datamodel.SecretStore, *datamodel.SecretStore)
	}{
		{
			name:     "default type is generic",
			testFile: testFileCertValueFrom,
			modifyResource: func(newResource, oldResource *datamodel.SecretStore) {
				newResource.Properties.Type = ""
			},
			assertions: func(t *testing.T, resp rest.Response, err error, newResource, oldResource *datamodel.SecretStore) {
				require.NoError(t, err)
				require.Nil(t, resp)
				require.Equal(t, datamodel.SecretTypeGeneric, newResource.Properties.Type)
			},
		},
		{
			name:     "new resource, but referencing valueFrom",
			testFile: testFileCertValueFrom,
			modifyResource: func(newResource, oldResource *datamodel.SecretStore) {
				newResource.Properties.Resource = ""
			},
			assertions: func(t *testing.T, resp rest.Response, err error, newResource, oldResource *datamodel.SecretStore) {
				require.NoError(t, err)
				r := resp.(*rest.BadRequestResponse)
				require.True(t, r.Body.Error.Message == "$.properties.data[tls.crt].Value must be given to create the secret." ||
					r.Body.Error.Message == "$.properties.data[tls.key].Value must be given to create the secret.")
			},
		},
		{
			name:        "update the existing resource - type not matched",
			testFile:    testFileCertValueFrom,
			oldResource: testutil.MustGetTestData[datamodel.SecretStore](testFileCertValueFrom),
			modifyResource: func(newResource, oldResource *datamodel.SecretStore) {
				oldResource.Properties.Type = datamodel.SecretTypeGeneric
			},
			assertions: func(t *testing.T, resp rest.Response, err error, newResource, oldResource *datamodel.SecretStore) {
				require.NoError(t, err)
				r := resp.(*rest.BadRequestResponse)
				require.Equal(t, "$.properties.type cannot change from 'generic' to 'certificate'.", r.Body.Error.Message)
			},
		},
		{
			name:        "inherit resource id from existing resource",
			testFile:    testFileCertValueFrom,
			oldResource: testutil.MustGetTestData[datamodel.SecretStore](testFileCertValueFrom),
			modifyResource: func(newResource, oldResource *datamodel.SecretStore) {
				newResource.Properties.Resource = ""
			},
			assertions: func(t *testing.T, resp rest.Response, err error, newResource, oldResource *datamodel.SecretStore) {
				require.NoError(t, err)
				require.Nil(t, resp)
				require.Equal(t, oldResource.Properties.Resource, newResource.Properties.Resource)
			},
		},
		{
			name:     "new basicAuthentication resource",
			testFile: testFileBasicAuthentication,
			assertions: func(t *testing.T, resp rest.Response, err error, newResource, oldResource *datamodel.SecretStore) {
				require.NoError(t, err)
				require.Nil(t, resp)
			},
		},
		{
			name:     "new awsIRSA resource",
			testFile: testFileAWSIRSA,
			assertions: func(t *testing.T, resp rest.Response, err error, newResource, oldResource *datamodel.SecretStore) {
				require.NoError(t, err)
				require.Nil(t, resp)
			},
		},
		{
			name:     "new azureWorkloadIdentity resource",
			testFile: testFileAzureWorkloadIdentity,
			assertions: func(t *testing.T, resp rest.Response, err error, newResource, oldResource *datamodel.SecretStore) {
				require.NoError(t, err)
				require.Nil(t, resp)
			},
		},
		{
			name:     "invalid basicAuthentication resource",
			testFile: testFileBasicAuthenticationInvalid,
			assertions: func(t *testing.T, resp rest.Response, err error, newResource, oldResource *datamodel.SecretStore) {
				require.NoError(t, err)
				r := resp.(*rest.BadRequestResponse)
				require.True(t, r.Body.Error.Message == "$.properties.data must contain 'password' key for basicAuthentication type.")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newResource := testutil.MustGetTestData[datamodel.SecretStore](tt.testFile)
			if tt.modifyResource != nil {
				tt.modifyResource(newResource, tt.oldResource)
			}
			resp, err := ValidateAndMutateRequest(context.TODO(), newResource, tt.oldResource, nil)
			tt.assertions(t, resp, err, newResource, tt.oldResource)
		})
	}
}

func TestUpsertSecret(t *testing.T) {
	t.Run("not found referenced key", func(t *testing.T) {
		newResource := testutil.MustGetTestData[datamodel.SecretStore](testFileCertValueFrom)

		ksecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "letsencrypt-prod",
				Namespace: "default",
			},
			Data: map[string][]byte{},
		}
		opt := &controller.Options{
			KubeClient: k8sutil.NewFakeKubeClient(nil, ksecret),
		}

		resp, err := UpsertSecret(context.TODO(), newResource, nil, opt)
		require.NoError(t, err)

		// assert
		r := resp.(*rest.BadRequestResponse)
		require.True(t, r.Body.Error.Message == "'default/letsencrypt-prod' resource does not have key, 'tls.crt'." ||
			r.Body.Error.Message == "'default/letsencrypt-prod' resource does not have key, 'tls.key'.")
	})

	t.Run("add secret values to the existing secret store", func(t *testing.T) {
		newResource := testutil.MustGetTestData[datamodel.SecretStore](testFileCertValue)
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
			KubeClient: k8sutil.NewFakeKubeClient(nil, ksecret),
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

	t.Run("inherit old resource id", func(t *testing.T) {
		oldResource := testutil.MustGetTestData[datamodel.SecretStore](testFileCertValueFrom)
		newResource := testutil.MustGetTestData[datamodel.SecretStore](testFileCertValueFrom)
		newResource.Properties.Resource = ""

		opt := &controller.Options{
			KubeClient: k8sutil.NewFakeKubeClient(nil),
		}

		_, err := UpsertSecret(context.TODO(), newResource, oldResource, opt)
		require.NoError(t, err)

		// assert
		require.Equal(t, oldResource.Properties.Resource, newResource.Properties.Resource)
	})

	t.Run("create new generic resource", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		sc := database.NewMockClient(ctrl)

		appData := testutil.MustGetTestData[any]("app_datamodel.json")

		sc.EXPECT().Get(gomock.Any(), testAppID, gomock.Any()).Return(&database.Object{
			Data: *appData,
		}, nil)

		newResource := testutil.MustGetTestData[datamodel.SecretStore](testFileGenericValue)
		newResource.Properties.Resource = ""

		opt := &controller.Options{
			DatabaseClient: sc,
			KubeClient:     k8sutil.NewFakeKubeClient(nil),
		}

		_, err := ValidateAndMutateRequest(context.TODO(), newResource, nil, opt)
		require.NoError(t, err)
		_, err = UpsertSecret(context.TODO(), newResource, nil, opt)
		require.NoError(t, err)

		// assert
		require.Equal(t, "app0-ns/secret0", newResource.Properties.Resource)
		ksecret := &corev1.Secret{}

		err = opt.KubeClient.Get(context.TODO(), runtimeclient.ObjectKey{Namespace: "app0-ns", Name: "secret0"}, ksecret)
		require.NoError(t, err)

		require.Equal(t, "dGxzLmNydA==", string(ksecret.Data["tls.crt"]))
		require.Equal(t, "dGxzLmNlcnQK", string(ksecret.Data["tls.key"]))
		require.Equal(t, "MTAwMDAwMDAtMTAwMC0xMDAwLTAwMDAtMDAwMDAwMDAwMDAw", string(ksecret.Data["servicePrincipalPassword"]))
		require.Equal(t, rpv1.OutputResource{
			LocalID: "Secret",
			ID: resources_kubernetes.IDFromParts(
				resources_kubernetes.PlaneNameTODO,
				"",
				resources_kubernetes.KindSecret,
				"app0-ns",
				"secret0"),
		}, newResource.Properties.Status.OutputResources[0])
	})

	t.Run("create new resource when namespace is missing", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		sc := database.NewMockClient(ctrl)

		appData := testutil.MustGetTestData[any]("app_datamodel.json")

		sc.EXPECT().Get(gomock.Any(), testAppID, gomock.Any()).Return(&database.Object{
			Data: *appData,
		}, nil)

		oldResource := testutil.MustGetTestData[datamodel.SecretStore](testFileCertValueFrom)
		oldResource.Properties.Resource = "app0-ns/secret0"
		newResource := testutil.MustGetTestData[datamodel.SecretStore](testFileCertValueFrom)
		newResource.Properties.Resource = "secret0"

		opt := &controller.Options{
			DatabaseClient: sc,
			KubeClient:     k8sutil.NewFakeKubeClient(nil),
		}

		_, err := ValidateAndMutateRequest(context.TODO(), newResource, nil, opt)
		require.NoError(t, err)
		_, err = UpsertSecret(context.TODO(), newResource, oldResource, opt)
		require.NoError(t, err)

		// assert
		require.Equal(t, "app0-ns/secret0", newResource.Properties.Resource)
	})

	t.Run("unmatched resource when namespace is missing in new resource", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		sc := database.NewMockClient(ctrl)

		appData := testutil.MustGetTestData[any]("app_datamodel.json")

		sc.EXPECT().Get(gomock.Any(), testAppID, gomock.Any()).Return(&database.Object{
			Data: *appData,
		}, nil)

		oldResource := testutil.MustGetTestData[datamodel.SecretStore](testFileCertValueFrom)
		oldResource.Properties.Resource = "app0-ns/secret0"
		newResource := testutil.MustGetTestData[datamodel.SecretStore](testFileCertValueFrom)
		newResource.Properties.Resource = "secret1"

		opt := &controller.Options{
			DatabaseClient: sc,
			KubeClient:     k8sutil.NewFakeKubeClient(nil),
		}

		_, err := ValidateAndMutateRequest(context.TODO(), newResource, nil, opt)
		require.NoError(t, err)
		resp, err := UpsertSecret(context.TODO(), newResource, oldResource, opt)
		require.NoError(t, err)

		// assert
		r := resp.(*rest.BadRequestResponse)
		require.Equal(t, "'app0-ns/secret1' of $.properties.resource must be same as 'app0-ns/secret0'.", r.Body.Error.Message)
	})

	t.Run("create a new secret resource with global scope", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		sc := database.NewMockClient(ctrl)

		newResource := testutil.MustGetTestData[datamodel.SecretStore](testFileGenericValueGlobalScope)

		opt := &controller.Options{
			DatabaseClient: sc,
			KubeClient:     k8sutil.NewFakeKubeClient(nil),
		}

		_, err := ValidateAndMutateRequest(context.TODO(), newResource, nil, opt)
		require.NoError(t, err)
		_, err = UpsertSecret(context.TODO(), newResource, nil, opt)
		require.NoError(t, err)

		// assert
		require.Equal(t, "test-namespace/secret0", newResource.Properties.Resource)
		ksecret := &corev1.Secret{}

		err = opt.KubeClient.Get(context.TODO(), runtimeclient.ObjectKey{Namespace: "test-namespace", Name: "secret0"}, ksecret)
		require.NoError(t, err)

		require.Equal(t, "dGxzLmNydA==", string(ksecret.Data["tls.crt"]))
		require.Equal(t, "dGxzLmNlcnQK", string(ksecret.Data["tls.key"]))
		require.Equal(t, "MTAwMDAwMDAtMTAwMC0xMDAwLTAwMDAtMDAwMDAwMDAwMDAw", string(ksecret.Data["servicePrincipalPassword"]))
		require.Equal(t, rpv1.OutputResource{
			LocalID: "Secret",
			ID: resources_kubernetes.IDFromParts(
				resources_kubernetes.PlaneNameTODO,
				"",
				resources_kubernetes.KindSecret,
				"test-namespace",
				"secret0"),
		}, newResource.Properties.Status.OutputResources[0])
	})

	t.Run("create a new secret resource with invalid resource", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		sc := database.NewMockClient(ctrl)

		newResource := testutil.MustGetTestData[datamodel.SecretStore](testFileGenericValueInvalidResource)

		opt := &controller.Options{
			DatabaseClient: sc,
			KubeClient:     k8sutil.NewFakeKubeClient(nil),
		}

		_, err := ValidateAndMutateRequest(context.TODO(), newResource, nil, opt)
		require.NoError(t, err)
		_, err = UpsertSecret(context.TODO(), newResource, nil, opt)
		require.Error(t, err)
		require.Equal(t, err.Error(), "no Kubernetes namespace")
	})

	t.Run("create a new secret resource with empty resource", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		sc := database.NewMockClient(ctrl)

		newResource := testutil.MustGetTestData[datamodel.SecretStore](testFileGenericValueEmptyResource)

		opt := &controller.Options{
			DatabaseClient: sc,
			KubeClient:     k8sutil.NewFakeKubeClient(nil),
		}

		_, err := ValidateAndMutateRequest(context.TODO(), newResource, nil, opt)
		require.NoError(t, err)
		resp, err := UpsertSecret(context.TODO(), newResource, nil, opt)
		require.NoError(t, err)

		// assert
		r := resp.(*rest.BadRequestResponse)
		require.Equal(t, "$.properties.resource cannot be empty for global scoped resource.", r.Body.Error.Message)
	})

	t.Run("add secret values to the existing secret store 1 ", func(t *testing.T) {
		newResource := testutil.MustGetTestData[datamodel.SecretStore](testFileCertValue)
		newResource.Properties.Resource = "default/secret"

		opt := &controller.Options{
			KubeClient: k8sutil.NewFakeKubeClient(nil),
		}

		resp, _ := UpsertSecret(context.TODO(), newResource, nil, opt)
		r := resp.(*rest.BadRequestResponse)
		require.Equal(t, "'default/secret' referenced resource does not exist.", r.Body.Error.Message)
	})

	t.Run("inherit old resource id for global scoped resource", func(t *testing.T) {
		oldResource := testutil.MustGetTestData[datamodel.SecretStore](testFileGenericValueGlobalScope)
		newResource := testutil.MustGetTestData[datamodel.SecretStore](testFileGenericValueEmptyResource)

		opt := &controller.Options{
			KubeClient: k8sutil.NewFakeKubeClient(nil),
		}

		_, err := UpsertSecret(context.TODO(), newResource, oldResource, opt)
		require.NoError(t, err)

		// assert
		require.Equal(t, oldResource.Properties.Resource, newResource.Properties.Resource)
	})
}

func TestDeleteSecret(t *testing.T) {
	t.Run("delete secret created by Radius", func(t *testing.T) {
		res := testutil.MustGetTestData[datamodel.SecretStore](testFileCertValueFrom)

		ksecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "letsencrypt-prod",
				Namespace: "default",
				Labels: map[string]string{
					kubernetes.LabelRadiusResourceType: "test",
				},
			},
			Data: map[string][]byte{},
		}
		opt := &controller.Options{
			KubeClient: k8sutil.NewFakeKubeClient(nil, ksecret),
		}

		resp, err := DeleteRadiusSecret(context.TODO(), res, opt)
		require.NoError(t, err)
		require.Nil(t, resp)

		err = opt.KubeClient.Get(context.TODO(), runtimeclient.ObjectKey{Namespace: "default", Name: "letsencrypt-prod"}, ksecret)
		require.True(t, apierrors.IsNotFound(err))
	})

	t.Run("not delete secret unless secret resource has radius label", func(t *testing.T) {
		res := testutil.MustGetTestData[datamodel.SecretStore](testFileCertValueFrom)

		ksecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "letsencrypt-prod",
				Namespace: "default",
			},
			Data: map[string][]byte{},
		}
		opt := &controller.Options{
			KubeClient: k8sutil.NewFakeKubeClient(nil, ksecret),
		}

		resp, err := DeleteRadiusSecret(context.TODO(), res, opt)
		require.NoError(t, err)
		require.Nil(t, resp)

		err = opt.KubeClient.Get(context.TODO(), runtimeclient.ObjectKey{Namespace: "default", Name: "letsencrypt-prod"}, ksecret)
		require.False(t, apierrors.IsNotFound(err))
	})
}
