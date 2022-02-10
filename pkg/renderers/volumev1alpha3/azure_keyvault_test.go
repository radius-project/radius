// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package volumev1alpha3

import (
	"sort"
	"testing"

	"github.com/Azure/go-autorest/autorest/to"
	"github.com/Azure/secrets-store-csi-driver-provider-azure/pkg/provider"
	"github.com/project-radius/radius/pkg/azure/radclient"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	csidriver "sigs.k8s.io/secrets-store-csi-driver/apis/v1alpha1"
)

func specToKeyVaultObjects(spec string) ([]provider.KeyVaultObject, error) {
	objects := map[string][]string{}
	err := yaml.Unmarshal([]byte(spec), &objects)
	if err != nil {
		return nil, err
	}

	keyVaultObjects := []provider.KeyVaultObject{}
	for _, o := range objects["array"] {
		var kvObject provider.KeyVaultObject
		err = yaml.Unmarshal([]byte(o), &kvObject)
		if err != nil {
			return nil, err
		}
		keyVaultObjects = append(keyVaultObjects, kvObject)
	}

	return keyVaultObjects, nil
}

type ByObjectName []provider.KeyVaultObject

func (a ByObjectName) Len() int           { return len(a) }
func (a ByObjectName) Less(i, j int) bool { return a[i].ObjectName < a[j].ObjectName }
func (a ByObjectName) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }

func Test_MakeSecretProviderClass(t *testing.T) {
	secrets := map[string]*radclient.SecretObjectProperties{
		"s1": {
			Name: to.StringPtr("secret1"),
		},
		"s2": {
			Name:     to.StringPtr("secret2"),
			Alias:    to.StringPtr("s2"),
			Encoding: radclient.SecretObjectPropertiesEncodingHex.ToPtr(),
			Version:  to.StringPtr("1"),
		},
	}
	keys := map[string]*radclient.KeyObjectProperties{
		"k1": {
			Name:    to.StringPtr("key1"),
			Alias:   to.StringPtr("k1"),
			Version: to.StringPtr("1"),
		},
	}

	certs := map[string]*radclient.CertificateObjectProperties{
		"c1": {
			Name:     to.StringPtr("cert1"),
			Alias:    to.StringPtr("c1"),
			Encoding: radclient.CertificateObjectPropertiesEncodingHex.ToPtr(),
			Version:  to.StringPtr("1"),
			Value:    radclient.CertificateObjectPropertiesValuePrivatekey.ToPtr(),
			Format:   radclient.CertificateObjectPropertiesFormatPem.ToPtr(),
		},
		"c2": {
			Name:  to.StringPtr("cert2"),
			Value: radclient.CertificateObjectPropertiesValuePublickey.ToPtr(),
		},
		"c3": {
			Name:  to.StringPtr("cert3"),
			Alias: to.StringPtr("c3"),
			Value: radclient.CertificateObjectPropertiesValueCertificate.ToPtr(),
		},
	}
	secretObjects := SecretObjects{
		secrets:      secrets,
		certificates: certs,
		keys:         keys,
	}
	or, err := makeSecretProviderClass("myapp", "testVolume", "fakeTenantID", "test-secretprovider", "/subscriptions/sub/resourceGroups/rg/providers/Microsoft.KeyVault/vaults/azure-kv", secretObjects)
	require.NoError(t, err)
	require.Equal(t, or.LocalID, outputresource.LocalIDSecretProviderClass)
	require.Equal(t, or.Identity, resourcemodel.ResourceIdentity{
		Kind: resourcemodel.IdentityKindKubernetes,
		Data: resourcemodel.KubernetesIdentity{
			Kind:       "SecretProviderClass",
			Name:       "test-secretprovider",
			Namespace:  "myapp",
			APIVersion: "secrets-store.csi.x-k8s.io/v1alpha1",
		},
	})
	require.Equal(t, outputresource.TypeKubernetes, or.ResourceKind)

	expectedSecretObjects := []provider.KeyVaultObject{
		{
			ObjectName:     "secret1",
			ObjectAlias:    *to.StringPtr("s1"),
			ObjectType:     "secret",
			ObjectVersion:  *to.StringPtr(""),
			ObjectEncoding: string(radclient.SecretObjectPropertiesEncodingUTF8),
		},
		{
			ObjectName:     "secret2",
			ObjectAlias:    *to.StringPtr("s2"),
			ObjectType:     "secret",
			ObjectVersion:  *to.StringPtr("1"),
			ObjectEncoding: string(radclient.SecretObjectPropertiesEncodingHex),
		},
		{
			ObjectName:    "key1",
			ObjectAlias:   *to.StringPtr("k1"),
			ObjectType:    "key",
			ObjectVersion: *to.StringPtr("1"),
		},
		{
			ObjectName:     "cert1",
			ObjectAlias:    *to.StringPtr("c1"),
			ObjectType:     "secret",
			ObjectVersion:  *to.StringPtr("1"),
			ObjectFormat:   string(radclient.CertificateObjectPropertiesFormatPem),
			ObjectEncoding: string(radclient.SecretObjectPropertiesEncodingHex),
		},
		{
			ObjectName:     "cert2",
			ObjectAlias:    *to.StringPtr("c2"),
			ObjectType:     "key",
			ObjectVersion:  *to.StringPtr(""),
			ObjectFormat:   string(radclient.CertificateObjectPropertiesFormatPfx),
			ObjectEncoding: "",
		},
		{
			ObjectName:     "cert3",
			ObjectAlias:    *to.StringPtr("c3"),
			ObjectType:     "certificate",
			ObjectVersion:  *to.StringPtr(""),
			ObjectFormat:   string(radclient.CertificateObjectPropertiesFormatPfx),
			ObjectEncoding: "",
		},
	}

	secretsSpec, err := getKeyVaultObjectsSpec(expectedSecretObjects)
	require.NoError(t, err)

	expectedSpec := csidriver.SecretProviderClass{
		TypeMeta: metav1.TypeMeta{
			Kind:       "SecretProviderClass",
			APIVersion: "secrets-store.csi.x-k8s.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secretprovider",
			Namespace: "myapp",
		},
		Spec: csidriver.SecretProviderClassSpec{
			Provider: "azure",
			Parameters: map[string]string{
				"usePodIdentity": "true",
				"keyvaultName":   "azure-kv",
				"objects":        secretsSpec,
				"tenantId":       "fakeTenantID",
			},
		},
	}

	actualSpec := or.Resource.(*csidriver.SecretProviderClass)
	actualSecretObjects, err := specToKeyVaultObjects(actualSpec.Spec.Parameters["objects"])
	require.NoError(t, err)
	require.Equal(t, expectedSpec.TypeMeta, actualSpec.TypeMeta)
	require.Equal(t, expectedSpec.ObjectMeta, actualSpec.ObjectMeta)
	require.Equal(t, "true", actualSpec.Spec.Parameters["usePodIdentity"])
	require.Equal(t, "azure-kv", actualSpec.Spec.Parameters["keyvaultName"])
	require.Equal(t, "fakeTenantID", actualSpec.Spec.Parameters["tenantId"])
	require.Equal(t, "azure", string(actualSpec.Spec.Provider))

	// The ordering of the secret objects could be different. For reliable comparison, sort
	sort.Sort(ByObjectName(expectedSecretObjects))
	sort.Sort(ByObjectName(actualSecretObjects))
	require.Equal(t, expectedSecretObjects, actualSecretObjects)

}
