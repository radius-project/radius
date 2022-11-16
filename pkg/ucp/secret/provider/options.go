// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package provider

import "github.com/project-radius/radius/pkg/ucp/dataprovider"

// SecretProviderOptions contains provider information of the secret.
type SecretProviderOptions struct {
	// Provider configures the secret provider.
	Provider SecretProviderType `yaml:"provider"`

	// ETCD configures options for the etcd secret store.
	ETCD dataprovider.ETCDOptions `yaml:"etcd,omitempty"`
}
