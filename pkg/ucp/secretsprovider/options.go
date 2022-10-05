// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package secretsprovider

import "github.com/project-radius/radius/pkg/ucp/dataprovider"

type SecretsProviderOptions struct {
	// Provider configures the storage provider.
	Provider SecretsProviderType `yaml:"provider"`

	// ETCD configures options for the etcd store.
	ETCD dataprovider.ETCDOptions `yaml:"etcd,omitempty"`
}
