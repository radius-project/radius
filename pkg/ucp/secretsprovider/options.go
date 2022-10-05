// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package secretsprovider

type SecretsProviderOptions struct {
	// Provider configures the storage provider.
	Provider SecretsProviderType `yaml:"provider"`
}