// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package secret

import (
	"encoding/json"

	ucp "github.com/project-radius/radius/pkg/ucp/api/v20220315privatepreview"
)

const (
	TestSecretId = "/planes/azure/azuresecrets/providers/System.Azure/credentials/default"
)

func GetTestAzureSecret() (ucp.AzureServicePrincipalProperties, error) {
	kind := "azure"
	clientId := "clientId"
	secretName := "secret"
	tenantId := "tenantId"
	storageKind := "kubernetes"
	secrets := ucp.AzureServicePrincipalProperties{
		Kind:     &kind,
		Storage:  &ucp.CredentialResourcePropertiesStorage{Kind: (*ucp.CredentialStorageKind)(&storageKind)},
		ClientID: &clientId,
		Secret:   &secretName,
		TenantID: &tenantId,
	}

	return secrets, nil
}

func GetTestAzureSecretResponse() ([]byte, error) {
	secret, err := GetTestAzureSecret()
	if err != nil {
		return nil, err
	}
	secretBytes, err := json.Marshal(secret)
	if err != nil {
		return nil, err
	}
	return secretBytes, nil
}
