// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package credentials

import (
	"context"

	ucp_dm "github.com/project-radius/radius/pkg/ucp/datamodel"
)

const (
	// AzureCloud represents the public cloud plane name for UCP.
	AzureCloud = "azurecloud"
)

type (
	// AzureCredential represents a credential for Azure AD.
	AzureCredential = ucp_dm.AzureCredentialProperties
	// AWSCredential represents a credential for AWS IAM.
	AWSCredential = ucp_dm.AWSCredentialProperties
)

// CredentialProvider is an UCP credential provider interface.
type CredentialProvider[T any] interface {
	// Fetch gets the credentials from secret storage.
	Fetch(ctx context.Context, planeName, name string) (*T, error)
}
