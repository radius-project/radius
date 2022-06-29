// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package provider

// QueueProviderType represents types of queue provider.
type QueueProviderType string

const (
	// TypeInmemory represents inmemory queue provider.
	TypeInmemory QueueProviderType = "inmemory"

	// TypeAPIServer represents the Kubernetes APIServer provider.
	TypeAPIServer QueueProviderType = "apiserver"
)
