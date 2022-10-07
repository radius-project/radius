// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package volume

import (
	"context"
	"errors"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	azcsi "github.com/Azure/secrets-store-csi-driver-provider-azure/pkg/provider/types"
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/renderers"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/rp/outputresource"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	csiv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"
)

var (
	errCreateSecretResource      = errors.New("unable to create secret provider class")
	errInvalidKeyVaultResourceID = errors.New("failed to parse KeyVault ResourceID. Unable to create secret provider class")
	errUnsupportedIdentityKind   = errors.New("unsupported identity kind")
)

var _ VolumeRenderer = (*AzureKeyvaultVolumeRenderer)(nil)

// AzureKeyvaultVolumeRenderer is the render to generate a SecretProviderClass resource.
type AzureKeyvaultVolumeRenderer struct {
}

// Render constructs a SecretProviderClass for Azure Key Vault CSI Driver volume
func (r *AzureKeyvaultVolumeRenderer) Render(ctx context.Context, resource conv.DataModelInterface, options *renderers.RenderOptions) (renderers.RendererOutput, error) {
	dm, ok := resource.(*datamodel.VolumeResource)
	if !ok {
		return renderers.RendererOutput{}, conv.ErrInvalidModelConversion
	}

	properties := dm.Properties.AzureKeyVault

	secretObjects := &SecretObjects{
		secrets:      properties.Secrets,
		certificates: properties.Certificates,
		keys:         properties.Keys,
	}

	appId, err := resources.ParseResource(dm.Properties.Application)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	secretProviderClassName := kubernetes.MakeResourceName(appId.Name(), dm.Name)
	outputResource, err := makeSecretProviderClass(options.Environment.Namespace, secretProviderClassName, properties.Resource, secretObjects, &dm.Properties.AzureKeyVault.Identity)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	return renderers.RendererOutput{
		Resources:      []outputresource.OutputResource{outputResource},
		ComputedValues: map[string]rp.ComputedValueReference{},
		SecretValues:   map[string]rp.SecretValueReference{},
	}, nil
}

func makeSecretProviderClass(namespace string, secretProviderName string, keyVaultResourceID string, secretObjects *SecretObjects, identity *datamodel.AzureIdentity) (outputresource.OutputResource, error) {
	keyVaultObjects := []azcsi.KeyVaultObject{}
	// Construct the spec for the secret objects
	for name, secret := range secretObjects.secrets {
		secretValues := getValuesOrDefaultsForSecrets(name, &secret)
		secretSpec := azcsi.KeyVaultObject{
			ObjectName:     secret.Name,
			ObjectAlias:    secretValues.alias,
			ObjectType:     "secret",
			ObjectVersion:  secretValues.version,
			ObjectEncoding: secretValues.encoding,
		}
		keyVaultObjects = append(keyVaultObjects, secretSpec)
	}

	for name, key := range secretObjects.keys {
		keyValues := getValuesOrDefaultsForKeys(name, &key)
		keySpec := azcsi.KeyVaultObject{
			ObjectName:    key.Name,
			ObjectAlias:   keyValues.alias,
			ObjectType:    "key",
			ObjectVersion: keyValues.version,
		}
		keyVaultObjects = append(keyVaultObjects, keySpec)
	}

	for name, cert := range secretObjects.certificates {
		certValues := getValuesOrDefaultsForCertificates(name, &cert)
		certSpec := azcsi.KeyVaultObject{
			ObjectName:     cert.Name,
			ObjectAlias:    certValues.alias,
			ObjectVersion:  certValues.version,
			ObjectEncoding: certValues.encoding,
			ObjectFormat:   certValues.format,
		}

		switch *cert.CertType {
		case datamodel.CertificateTypeCertificate:
			// Setting objectType: cert will fetch and write only the certificate from keyvault.
			certSpec.ObjectType = "certificate"
		case datamodel.CertificateTypePublicKey:
			// Setting objectType: key will fetch and write only the public key from keyvault.
			certSpec.ObjectType = "key"
		case datamodel.CertificateTypePrivateKey:
			// Setting objectType: secret will fetch and write the certificate and private key from keyvault.
			// The private key and certificate are written to a single file.
			certSpec.ObjectType = "secret"
		}

		keyVaultObjects = append(keyVaultObjects, certSpec)
	}

	keyVaultObjectsSpec, err := getKeyVaultObjectsSpec(keyVaultObjects)
	if err != nil {
		return outputresource.OutputResource{}, errCreateSecretResource
	}

	kvResourceID, err := resources.ParseResource(keyVaultResourceID)
	if err != nil {
		return outputresource.OutputResource{}, errInvalidKeyVaultResourceID
	}

	params := map[string]string{
		"usePodIdentity": "false",
		"clientID":       identity.ClientID,
		"tenantID":       identity.TenantID,
		"keyvaultName":   kvResourceID.Name(),
		"objects":        keyVaultObjectsSpec,
	}

	switch identity.Kind {
	case datamodel.AzureIdentitySystemAssigned:
		// https://azure.github.io/secrets-store-csi-driver-provider-azure/docs/configurations/identity-access-modes/system-assigned-msi-mode/
		params["useVMManagedIdentity"] = "true"
		// clientID must be empty for system assigned managed identity
		params["clientID"] = ""
		// tenantID is a fake id to bypass crd validation because CSI doesn't require a tenant ID for System/User assigned managed identity.
		params["tenantID"] = "placeholder"

	default:
		return outputresource.OutputResource{}, errUnsupportedIdentityKind
	}

	secretProvider := csiv1.SecretProviderClass{
		TypeMeta: metav1.TypeMeta{
			Kind:       "SecretProviderClass",
			APIVersion: "secrets-store.csi.x-k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretProviderName,
			Namespace: namespace,
		},
		Spec: csiv1.SecretProviderClassSpec{
			Provider:   "azure",
			Parameters: params,
		},
	}

	return outputresource.NewKubernetesOutputResource(resourcekinds.SecretProviderClass, outputresource.LocalIDSecretProviderClass, &secretProvider, secretProvider.ObjectMeta), nil
}

func getValuesOrDefaultsForSecrets(name string, secretObject *datamodel.SecretObjectProperties) objectValues {
	alias := secretObject.Alias
	if alias == "" {
		alias = name
	}

	version := secretObject.Version
	encoding := to.Ptr(datamodel.SecretObjectPropertiesEncodingUTF8)
	if secretObject.Encoding != nil {
		encoding = secretObject.Encoding
	}

	return objectValues{
		alias:    alias,
		version:  version,
		encoding: string(*encoding),
	}
}

func getValuesOrDefaultsForKeys(name string, keyObject *datamodel.KeyObjectProperties) objectValues {
	alias := keyObject.Alias
	if alias == "" {
		alias = name
	}

	version := keyObject.Version

	return objectValues{
		alias:   alias,
		version: version,
	}
}

func getValuesOrDefaultsForCertificates(name string, certificateObject *datamodel.CertificateObjectProperties) objectValues {
	alias := certificateObject.Alias
	version := certificateObject.Version

	// CSI driver supports object encoding only when object type = secret i.e. cert value is privatekey
	encoding := ""
	if *certificateObject.CertType == datamodel.CertificateTypePrivateKey {
		if certificateObject.Encoding == nil {
			encoding = string(datamodel.SecretObjectPropertiesEncodingUTF8)
		} else {
			encoding = string(*certificateObject.Encoding)
		}
	}

	format := string(datamodel.CertificateFormatPFX)
	if certificateObject.Format != nil {
		format = string(*certificateObject.Format)
	}

	return objectValues{
		alias:    alias,
		version:  version,
		encoding: encoding,
		format:   format,
	}
}

func getKeyVaultObjectsSpec(keyVaultObjects []azcsi.KeyVaultObject) (string, error) {
	// Azure Keyvault CSI driver accepts only array property for keyvault objects.
	yamlArray := azcsi.StringArray{Array: []string{}}

	for _, object := range keyVaultObjects {
		obj, err := yaml.Marshal(object)
		if err != nil {
			return "", errCreateSecretResource
		}
		yamlArray.Array = append(yamlArray.Array, string(obj))
	}

	objects, err := yaml.Marshal(yamlArray)
	if err != nil {
		return "", errCreateSecretResource
	}

	return string(objects), nil
}
