// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package volume

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	azcsi "github.com/Azure/secrets-store-csi-driver-provider-azure/pkg/provider/types"
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/azure/armauth"
	azclients "github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/renderers"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/rp/outputresource"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	csiv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"
)

var (
	errCreateSecretResource      = errors.New("unable to create secret provider class")
	errInvalidKeyVaultResourceID = errors.New("failed to parse KeyVault ResourceID. Unable to create secret provider class")
	errUnsupportedIdentityKind   = errors.New("unsupported identity kind")
	errInvalidManagedIdentityID  = errors.New("invalid managed identity resource id")
)

var _ VolumeRenderer = (*AzureKeyvaultVolumeRenderer)(nil)

// AzureKeyvaultVolumeRenderer is the render to generate a SecretProviderClass resource.
type AzureKeyvaultVolumeRenderer struct {
	Arm *armauth.ArmConfig
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

	// TODO: Move it to frontend.
	_, err := resources.ParseResource(dm.Properties.Application)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	outputResource, err := r.makeSecretProviderClass(ctx, options.Environment.Namespace, secretObjects, dm)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	resources := []outputresource.OutputResource{outputResource}
	computedValues := map[string]rp.ComputedValueReference{}
	if properties.Identity.Kind == datamodel.AzureIdentityWorkload {
		provider, ok := outputResource.Resource.(*csiv1.SecretProviderClass)
		if !ok {
			return renderers.RendererOutput{}, errors.New("failed to get ServiceProviderClass")
		}
		serviceResource, saName, err := r.makeServiceAccount(options.Environment.Namespace, dm, provider)
		if err != nil {
			return renderers.RendererOutput{}, err
		}
		resources = append(resources, *serviceResource)
		computedValues = map[string]rp.ComputedValueReference{
			"identity": {
				Value: dm.Properties.AzureKeyVault.Identity.Resource,
			},
			"identitySubject": {
				Value: fmt.Sprintf("system:serviceaccount:%s:%s", options.Environment.Namespace, saName),
			},
			"identityIssuer": {
				Value: dm.Properties.AzureKeyVault.Identity.Issuer,
			},
			"identityAudience": {
				Value: "api://AzureADTokenExchange",
			},
		}
	}

	return renderers.RendererOutput{
		Resources:      resources,
		ComputedValues: computedValues,
		SecretValues:   map[string]rp.SecretValueReference{},
	}, nil
}

func (r *AzureKeyvaultVolumeRenderer) makeServiceAccount(namespace string, res *datamodel.VolumeResource, provider *csiv1.SecretProviderClass) (*outputresource.OutputResource, string, error) {
	clientID, ok := provider.Spec.Parameters["clientID"]
	if !ok {
		return nil, "", errors.New("failed to get ClientID")
	}

	tenantID, ok := provider.Spec.Parameters["tenantID"]
	if !ok {
		return nil, "", errors.New("failed to get TenantID")
	}

	name := kubernetes.MakeResourceName(res.Properties.Application, res.Name)
	labels := kubernetes.MakeDescriptiveLabels(res.Properties.Application, res.Name, res.Type)
	labels["azure.workload.identity/use"] = "true"
	sa := &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ServiceAccount",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubernetes.MakeResourceName(res.Properties.Application, res.Name),
			Namespace: namespace,
			Labels:    labels,
			Annotations: map[string]string{
				"azure.workload.identity/client-id": clientID,
				"azure.workload.identity/tenant-id": tenantID,
			},
		},
	}

	outRes := outputresource.NewKubernetesOutputResource(resourcekinds.ServiceAccount, outputresource.LocalIDSecretProviderClass, sa, sa.ObjectMeta)

	return &outRes, name, nil

}

func (r *AzureKeyvaultVolumeRenderer) makeSecretProviderClass(ctx context.Context, namespace string, secretObjects *SecretObjects, res *datamodel.VolumeResource) (outputresource.OutputResource, error) {
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

	prop := res.Properties.AzureKeyVault

	kvResourceID, err := resources.ParseResource(prop.Resource)
	if err != nil {
		return outputresource.OutputResource{}, errInvalidKeyVaultResourceID
	}

	params := map[string]string{
		"usePodIdentity": "false",
		"keyvaultName":   kvResourceID.Name(),
		"objects":        keyVaultObjectsSpec,
	}

	switch prop.Identity.Kind {
	case datamodel.AzureIdentitySystemAssigned:
		// https://azure.github.io/secrets-store-csi-driver-provider-azure/docs/configurations/identity-access-modes/system-assigned-msi-mode/
		params["useVMManagedIdentity"] = "true"
		// clientID must be empty for system assigned managed identity
		params["clientID"] = ""
		// tenantID is a fake id to bypass crd validation because CSI doesn't require a tenant ID for System/User assigned managed identity.
		params["tenantID"] = "placeholder"

	case datamodel.AzureIdentityWorkload:
		rID, err := resources.ParseResource(prop.Identity.Resource)
		if err != nil {
			return outputresource.OutputResource{}, errInvalidManagedIdentityID
		}
		subscription := rID.FindScope(resources.SubscriptionsSegment)
		if subscription == "" {
			return outputresource.OutputResource{}, errInvalidManagedIdentityID
		}

		miClient := azclients.NewUserAssignedIdentitiesClient(subscription, r.Arm.Auth)
		mi, err := miClient.Get(ctx, rID.FindScope(resources.ResourceGroupsSegment), rID.Name())
		if err != nil {
			return outputresource.OutputResource{}, err
		}
		params["useVMManagedIdentity"] = "true"
		params["clientID"] = mi.ClientID.String()
		params["tenantID"] = mi.TenantID.String()

	default:
		return outputresource.OutputResource{}, errUnsupportedIdentityKind
	}

	secretProvider := &csiv1.SecretProviderClass{
		TypeMeta: metav1.TypeMeta{
			Kind:       "SecretProviderClass",
			APIVersion: "secrets-store.csi.x-k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubernetes.MakeResourceName(res.Properties.Application, res.Name),
			Namespace: namespace,
			Labels:    kubernetes.MakeDescriptiveLabels(res.Properties.Application, res.Name, res.Type),
		},
		Spec: csiv1.SecretProviderClassSpec{
			Provider:   "azure",
			Parameters: params,
		},
	}

	return outputresource.NewKubernetesOutputResource(resourcekinds.SecretProviderClass, outputresource.LocalIDSecretProviderClass, secretProvider, secretProvider.ObjectMeta), nil
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
