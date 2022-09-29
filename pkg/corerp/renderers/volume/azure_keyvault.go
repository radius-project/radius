// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package volume

import (
	"context"
	"fmt"

	azcsi "github.com/Azure/secrets-store-csi-driver-provider-azure/pkg/provider/types"
	"github.com/google/uuid"
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/rp/outputresource"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	csiv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"
)

// Possible values for certificate value field
const (
	CertificateValueCertificate = "certificate"
	CertificateValuePrivateKey  = "privatekey"
	CertificateValuePublicKey   = "publickey"
)

// SecretObjects wraps the different secret objects to be configured on the SecretProvider class
type SecretObjects struct {
	secrets      map[string]datamodel.SecretObjectProperties
	certificates map[string]datamodel.CertificateObjectProperties
	keys         map[string]datamodel.KeyObjectProperties
}

func getTenantID(ctx context.Context, arm armauth.ArmConfig) (string, error) {
	sc := clients.NewSubscriptionsClient(arm.Auth)
	s, err := sc.Get(ctx, arm.SubscriptionID)
	if err != nil {
		return "", fmt.Errorf("unable to find subscription: %w", err)
	}
	tenantID, err := uuid.Parse(*s.TenantID)
	if err != nil {
		return "", fmt.Errorf("failed to convert tenantID to UUID: %w", err)
	}
	return tenantID.String(), err
}

// GetAzureKeyVaultVolume constructs a SecretProviderClass for Azure Key Vault CSI Driver volume
func GetAzureKeyVaultVolume(ctx context.Context, arm *armauth.ArmConfig, resource conv.DataModelInterface, options *renderers.RenderOptions) (renderers.RendererOutput, error) {
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

	secretProviderClassName := dm.Name + "-sp"
	tenantID, err := getTenantID(ctx, *arm)
	if err != nil {
		return renderers.RendererOutput{}, fmt.Errorf("Unable to construct secret provider class. Failed to get tenant ID")
	}
	outputResource, err := makeSecretProviderClass(options.Environment.Namespace, tenantID, secretProviderClassName, properties.Resource, secretObjects)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	return renderers.RendererOutput{
		Resources:      []outputresource.OutputResource{outputResource},
		ComputedValues: map[string]rp.ComputedValueReference{},
		SecretValues:   map[string]rp.SecretValueReference{},
	}, nil
}

type objectValues struct {
	alias    string
	version  string
	encoding string
	format   string
}

func getValuesOrDefaultsForSecrets(name string, secretObject *datamodel.SecretObjectProperties) objectValues {
	var alias, version, encoding, format string
	if secretObject.Alias == "" {
		alias = name
	} else {
		alias = secretObject.Alias
	}

	if secretObject.Version == "" {
		version = ""
	} else {
		version = secretObject.Version
	}

	if secretObject.Encoding == nil {
		encoding = string(datamodel.SecretObjectPropertiesEncodingUTF8)
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

func getValuesOrDefaultsForKeys(name string, keyObject *datamodel.KeyObjectProperties) objectValues {
	var alias, version, encoding, format string
	if keyObject.Alias == "" {
		alias = name
	} else {
		alias = keyObject.Alias
	}

	if keyObject.Version == "" {
		version = ""
	} else {
		version = keyObject.Version
	}

	return objectValues{
		alias:    alias,
		version:  version,
		encoding: encoding,
		format:   format,
	}
}

func getValuesOrDefaultsForCertificates(name string, certificateObject *datamodel.CertificateObjectProperties) objectValues {
	var alias, version, encoding, format string
	if certificateObject.Alias == "" {
		alias = name
	} else {
		alias = certificateObject.Alias
	}

	if certificateObject.Version == "" {
		version = ""
	} else {
		version = certificateObject.Version
	}

	// CSI driver supports object encoding only when object type = secret i.e. cert value is privatekey
	encoding = ""
	if *certificateObject.CertType == datamodel.CertificateTypePrivateKey {
		if certificateObject.Encoding == nil {
			encoding = string(datamodel.SecretObjectPropertiesEncodingUTF8)
		} else {
			encoding = string(*certificateObject.Encoding)
		}
	}

	if certificateObject.Format == nil {
		format = string(datamodel.CertificateFormatPFX)
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

func makeSecretProviderClass(namespace string, tenantID string, secretProviderName string, keyVaultResourceID string, secretObjects *SecretObjects) (outputresource.OutputResource, error) {
	// Make SecretProvider class
	// https://azure.github.io/secrets-store-csi-driver-provider-azure/getting-started/usage/#create-your-own-secretproviderclass-object

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
		var certSpec azcsi.KeyVaultObject
		certValues := getValuesOrDefaultsForCertificates(name, &cert)
		switch *cert.CertType {
		case datamodel.CertificateTypeCertificate:
			certSpec = azcsi.KeyVaultObject{
				ObjectName:     cert.Name,
				ObjectAlias:    certValues.alias,
				ObjectType:     "certificate", // Setting objectType: cert will fetch and write only the certificate from keyvault
				ObjectVersion:  certValues.version,
				ObjectEncoding: certValues.encoding,
				ObjectFormat:   certValues.format,
			}
		case datamodel.CertificateTypePublicKey:

			certSpec = azcsi.KeyVaultObject{
				ObjectName:     cert.Name,
				ObjectAlias:    certValues.alias,
				ObjectType:     "key", // Setting objectType: key will fetch and write only the public key from keyvault
				ObjectVersion:  certValues.version,
				ObjectEncoding: certValues.encoding,
				ObjectFormat:   certValues.format,
			}
		case datamodel.CertificateTypePrivateKey:
			certSpec = azcsi.KeyVaultObject{
				ObjectName:     cert.Name,
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

	kvResourceID, err := resources.ParseResource(keyVaultResourceID)
	if err != nil {
		return outputresource.OutputResource{}, fmt.Errorf("Failed to parse KeyVault ResourceID. Unable to create secret provider class")
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
			Provider: "azure",
			Parameters: map[string]string{
				"usePodIdentity": "true",
				"keyvaultName":   kvResourceID.Name(),
				"objects":        keyVaultObjectsSpec,
			},
		},
	}

	return outputresource.NewKubernetesOutputResource(resourcekinds.SecretProviderClass, outputresource.LocalIDSecretProviderClass, &secretProvider, secretProvider.ObjectMeta), nil
}

func getKeyVaultObjectsSpec(keyVaultObjects []azcsi.KeyVaultObject) (string, error) {
	yamlArray := azcsi.StringArray{Array: []string{}}
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
