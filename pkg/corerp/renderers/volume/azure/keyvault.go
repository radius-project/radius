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

package azure

import (
	"context"
	"errors"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/renderers"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/to"

	azcsi "github.com/Azure/secrets-store-csi-driver-provider-azure/pkg/provider/types"
	"gopkg.in/yaml.v3"
)

const (
	// SPCVolumeObjectSpecKey represents the key of volume resource computedValues to keep the parameters for SecretProviderClass.
	SPCVolumeObjectSpecKey = "csiobjectspec"
)

var (
	errCreateSecretResource = errors.New("unable to create secret provider class")
)

// SecretObjects wraps the different secret objects to be configured on the SecretProvider class
type SecretObjects struct {
	secrets      map[string]datamodel.SecretObjectProperties
	certificates map[string]datamodel.CertificateObjectProperties
	keys         map[string]datamodel.KeyObjectProperties
}

type objectValues struct {
	alias    string
	version  string
	encoding string
	format   string
}

// KeyVaultRenderer is a render for azure keyvault volume.
type KeyVaultRenderer struct {
}

func (r *KeyVaultRenderer) Render(ctx context.Context, resource v1.DataModelInterface, options *renderers.RenderOptions) (*renderers.RendererOutput, error) {
	dm, ok := resource.(*datamodel.VolumeResource)
	if !ok {
		return nil, v1.ErrInvalidModelConversion
	}

	properties := dm.Properties.AzureKeyVault

	secretObjects := &SecretObjects{
		secrets:      properties.Secrets,
		certificates: properties.Certificates,
		keys:         properties.Keys,
	}
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
		return nil, errCreateSecretResource
	}

	computedValues := map[string]rpv1.ComputedValueReference{
		SPCVolumeObjectSpecKey: {
			Value: keyVaultObjectsSpec,
		},
	}

	return &renderers.RendererOutput{
		Resources:      []rpv1.OutputResource{},
		ComputedValues: computedValues,
		SecretValues:   map[string]rpv1.SecretValueReference{},
	}, nil
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
