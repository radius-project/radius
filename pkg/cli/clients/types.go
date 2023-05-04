// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package clients

type Providers struct {
	// Azure represents the configuration for the Azure IAC provider used during deployment. This field is optional.
	Azure *AzureProvider
	AWS   *AWSProvider
}

type AzureProvider struct {
	Scope string
}

type AWSProvider struct {
	Scope string
}
