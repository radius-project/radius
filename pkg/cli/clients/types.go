// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package clients

type Providers struct {
	// Azure provider information. This field is optional.
	Azure *AzureProvider
	// AWS provider information. This field is optional.
	AWS *AWSProvider
	// Radius provider information.
	Radius *RadiusProvider
}

type AzureProvider struct {
	// Scope is the target level for deploying the Azure resources.
	Scope string
}

type AWSProvider struct {
	// Scope is the target level for deploying the AWS resources.
	Scope string
}

type RadiusProvider struct {
	// Currently, we must provide an environment ID for deploying applications.
	EnvironmentID string
	// ApplicationID is the ID of the application to be deployed. This is optional.
	ApplicationID string
}
