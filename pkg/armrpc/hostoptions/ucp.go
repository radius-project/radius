// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package hostoptions

// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

type UCPConnectionKind = string

const (
	// UCPConnectionKindDirect describes a direct connection to UCP. See pkg/sdk/NewDirectConnection.
	UCPConnectionKindDirect UCPConnectionKind = "direct"

	// KindKubernetes describes a Kubernetes connection to UCP. See pkg/sdk/NewKubernetesConnectionFromConfig.
	UCPConnectionKindKubernetes UCPConnectionKind = "kubernetes"
)

// Config represents the configuration for a UCP connection inside our host
// configuration file.
type UCPConfig struct {
	// Kind describes the kind of connection. Use UCPConnectionKindKubernetes for production and UCPConnectionKindDirect for testing with
	// a standalone UCP process.
	Kind UCPConnectionKind `yaml:"kind"`

	// Direct describes the connection options for a direct connection.
	Direct *UCPDirectConnectionConfig `yaml:"direct,omitempty"`
}

// DirectConnectionConfig describes the connection options for a direct connection.
type UCPDirectConnectionConfig struct {
	// Endpoint is the URL endpoint for the connection.
	Endpoint string `yaml:"endpoint"`
}
