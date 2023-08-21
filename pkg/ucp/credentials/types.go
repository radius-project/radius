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

package credentials

import (
	"context"

	ucp_dm "github.com/radius-project/radius/pkg/ucp/datamodel"
)

const (
	// AzureCloud represents the public cloud plane name for UCP.
	AzureCloud = "azurecloud"

	// AWSPublic represents the aws public cloud plane name for UCP.
	AWSPublic = "aws"
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
