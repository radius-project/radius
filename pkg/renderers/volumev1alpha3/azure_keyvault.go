// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package volumev1alpha3

import (
	"context"
	"fmt"

	"github.com/Azure/secrets-store-csi-driver-provider-azure/pkg/provider"
	"github.com/gofrs/uuid"
	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/azure/radclient"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	csidriver "sigs.k8s.io/secrets-store-csi-driver/apis/v1alpha1"
)

// Possible values for certificate value field
const (
	CertificateValueCertificate = "certificate"
	CertificateValuePrivateKey  = "privatekey"
	CertificateValuePublicKey   = "publickey"
)

// SecretObjects wraps the different secret objects to be configured on the SecretProvider class
type SecretObjects struct {
	secrets      map[string]*radclient.SecretObjectProperties
	certificates map[string]*radclient.CertificateObjectProperties
	keys         map[string]*radclient.KeyObjectProperties
}

func getTenantID(ctx context.Context, arm armauth.ArmConfig) (string, error) {
	sc := clients.NewSubscriptionsClient(arm.Auth)
	s, err := sc.Get(ctx, arm.SubscriptionID)
	if err != nil {
		return "", fmt.Errorf("unable to find subscription: %w", err)
	}
	tenantID, err := uuid.FromString(*s.TenantID)
	if err != nil {
		return "", fmt.Errorf("failed to convert tenantID to UUID: %w", err)
	}
	return tenantID.String(), err
}

// GetAzureKeyVaultVolume constructs a SecretProviderClass for Azure Key Vault CSI Driver volume
func GetAzureKeyVaultVolume(ctx context.Context, arm *armauth.ArmConfig, resource renderers.RendererResource, dependencies map[string]renderers.RendererDependency) (renderers.RendererOutput, error) {
	properties := radclient.AzureKeyVaultVolumeProperties{}
	err := resource.ConvertDefinition(&properties)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	secretObjects := SecretObjects{
		secrets:      properties.Secrets,
		certificates: properties.Certificates,
		keys:         properties.Keys,
	}

	secretProviderClassName := resource.ResourceName + "-sp"
	tenantID, err := getTenantID(ctx, *arm)
	if err != nil {
		return renderers.RendererOutput{}, fmt.Errorf("Unable to construct secret provider class. Failed to get tenant ID")
	}
	outputResource, err := makeSecretProviderClass(resource.ApplicationName, resource.ResourceName, tenantID, secretProviderClassName, *properties.Resource, secretObjects)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	return renderers.RendererOutput{
		Resources:      []outputresource.OutputResource{outputResource},
		ComputedValues: map[string]renderers.ComputedValueReference{},
		SecretValues:   map[string]renderers.SecretValueReference{},
	}, nil
}

type objectValues struct {
	alias    string
	version  string
	encoding string
	format   string
}

func getValuesOrDefaultsForSecrets(name string, secretObject *radclient.SecretObjectProperties) objectValues {
	var alias, version, encoding, format string
	if secretObject.Alias == nil {
		alias = name
	} else {
		alias = *secretObject.Alias
	}

	if secretObject.Version == nil {
		version = ""
	} else {
		version = *secretObject.Version
	}

	if secretObject.Encoding == nil {
		encoding = string(radclient.CertificateObjectPropertiesEncodingUTF8)
	} else {
		encoding = string(*secretObject.Encoding)
	}

	return objectValues{
		alias:    alias,
		version:  version,
		encoding: encoding,
		format:   format,
	}
}

func getValuesOrDefaultsForKeys(name string, keyObject *radclient.KeyObjectProperties) objectValues {
	var alias, version, encoding, format string
	if keyObject.Alias == nil {
		alias = name
	} else {
		alias = *keyObject.Alias
	}

	if keyObject.Version == nil {
		version = ""
	} else {
		version = *keyObject.Version
	}

	return objectValues{
		alias:    alias,
		version:  version,
		encoding: encoding,
		format:   format,
	}
}

func getValuesOrDefaultsForCertificates(name string, certificateObject *radclient.CertificateObjectProperties) objectValues {
	var alias, version, encoding, format string
	if certificateObject.Alias == nil {
		alias = name
	} else {
		alias = *certificateObject.Alias
	}

	if certificateObject.Version == nil {
		version = ""
	} else {
		version = *certificateObject.Version
	}

	// CSI driver supports object encoding only when object type = secret i.e. cert value is privatekey
	encoding = ""
	if *certificateObject.Value == radclient.CertificateObjectPropertiesValuePrivatekey {
		if certificateObject.Encoding == nil {
			encoding = string(radclient.CertificateObjectPropertiesEncodingUTF8)
		} else {
			encoding = string(*certificateObject.Encoding)
		}
	}

	if certificateObject.Format == nil {
		format = string(radclient.CertificateObjectPropertiesFormatPfx)
	} else {
		format = string(*certificateObject.Format)
	}

	return objectValues{
		alias:    alias,
		version:  version,
		encoding: encoding,
		format:   format,
	}
}

func makeSecretProviderClass(appName string, volumeName string, tenantID string, secretProviderName string, keyVaultResourceID string, secretObjects SecretObjects) (outputresource.OutputResource, error) {
	// Make SecretProvider class
	// https://azure.github.io/secrets-store-csi-driver-provider-azure/getting-started/usage/#create-your-own-secretproviderclass-object

	keyVaultObjects := []provider.KeyVaultObject{}
	// Construct the spec for the secret objects
	for name, secret := range secretObjects.secrets {
		secretValues := getValuesOrDefaultsForSecrets(name, secret)
		secretSpec := provider.KeyVaultObject{
			ObjectName:     *secret.Name,
			ObjectAlias:    secretValues.alias,
			ObjectType:     "secret",
			ObjectVersion:  secretValues.version,
			ObjectEncoding: secretValues.encoding,
		}
		keyVaultObjects = append(keyVaultObjects, secretSpec)
	}
	for name, key := range secretObjects.keys {
		keyValues := getValuesOrDefaultsForKeys(name, key)
		keySpec := provider.KeyVaultObject{
			ObjectName:    *key.Name,
			ObjectAlias:   keyValues.alias,
			ObjectType:    "key",
			ObjectVersion: keyValues.version,
		}
		keyVaultObjects = append(keyVaultObjects, keySpec)
	}
	for name, cert := range secretObjects.certificates {
		var certSpec provider.KeyVaultObject
		certValues := getValuesOrDefaultsForCertificates(name, cert)
		switch *cert.Value {
		case radclient.CertificateObjectPropertiesValueCertificate:
			certSpec = provider.KeyVaultObject{
				ObjectName:     *cert.Name,
				ObjectAlias:    certValues.alias,
				ObjectType:     "certificate", // Setting objectType: cert will fetch and write only the certificate from keyvault
				ObjectVersion:  certValues.version,
				ObjectEncoding: certValues.encoding,
				ObjectFormat:   certValues.format,
			}
		case radclient.CertificateObjectPropertiesValuePublickey:

			certSpec = provider.KeyVaultObject{
				ObjectName:     *cert.Name,
				ObjectAlias:    certValues.alias,
				ObjectType:     "key", // Setting objectType: key will fetch and write only the public key from keyvault
				ObjectVersion:  certValues.version,
				ObjectEncoding: certValues.encoding,
				ObjectFormat:   certValues.format,
			}
		case radclient.CertificateObjectPropertiesValuePrivatekey:
			certSpec = provider.KeyVaultObject{
				ObjectName:     *cert.Name,
				ObjectAlias:    certValues.alias,
				ObjectType:     "secret", // Setting objectType: secret will fetch and write the certificate and private key from keyvault. The private key and certificate are written to a single file.
				ObjectVersion:  certValues.version,
				ObjectEncoding: certValues.encoding,
				ObjectFormat:   certValues.format,
			}
		}
		keyVaultObjects = append(keyVaultObjects, certSpec)
	}

	keyVaultObjectsSpec, err := getKeyVaultObjectsSpec(keyVaultObjects)
	if err != nil {
		return outputresource.OutputResource{}, fmt.Errorf("Unable to create secret provider class")
	}

	kvResourceID, err := azresources.Parse(keyVaultResourceID)
	if err != nil {
		return outputresource.OutputResource{}, fmt.Errorf("Failed to parse KeyVault ResourceID. Unable to create secret provider class")
	}
	secretProvider := csidriver.SecretProviderClass{
		TypeMeta: metav1.TypeMeta{
			Kind:       "SecretProviderClass",
			APIVersion: "secrets-store.csi.x-k8s.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretProviderName,
			Namespace: appName,
		},
		Spec: csidriver.SecretProviderClassSpec{
			Provider: "azure",
			Parameters: map[string]string{
				"usePodIdentity": "true",
				"keyvaultName":   kvResourceID.Name(),
				"objects":        keyVaultObjectsSpec,
				"tenantId":       tenantID,
			},
		},
	}

	return outputresource.NewKubernetesOutputResource(outputresource.LocalIDSecretProviderClass, &secretProvider, secretProvider.ObjectMeta), nil
}

func getKeyVaultObjectsSpec(keyVaultObjects []provider.KeyVaultObject) (string, error) {
	yamlArray := provider.StringArray{Array: []string{}}
	for _, object := range keyVaultObjects {
		obj, err := yaml.Marshal(object)
		if err != nil {
			return "", fmt.Errorf("Unable to create secret provider class")
		}
		yamlArray.Array = append(yamlArray.Array, string(obj))
	}

	objects, err := yaml.Marshal(yamlArray)
	if err != nil {
		return "", fmt.Errorf("Unable to create secret provider class")
	}
	return string(objects), nil
}
