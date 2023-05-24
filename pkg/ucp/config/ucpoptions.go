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

package config

import (
	"errors"

	"github.com/project-radius/radius/pkg/sdk"
	"k8s.io/client-go/rest"
)

type UCPConnectionKind = string

const (
	// UCPConnectionKindDirect describes a direct connection to UCP. See pkg/sdk/NewDirectConnection.
	UCPConnectionKindDirect UCPConnectionKind = "direct"

	// KindKubernetes describes a Kubernetes connection to UCP. See pkg/sdk/NewKubernetesConnectionFromConfig.
	UCPConnectionKindKubernetes UCPConnectionKind = "kubernetes"
)

// UCPOptions represents the configuration for a UCP connection inside our host
// configuration file.
type UCPOptions struct {
	// Kind describes the kind of connection. Use UCPConnectionKindKubernetes for production and UCPConnectionKindDirect for testing with
	// a standalone UCP process.
	Kind UCPConnectionKind `yaml:"kind"`

	// Direct describes the connection options for a direct connection.
	Direct *UCPDirectConnectionOptions `yaml:"direct,omitempty"`
}

// UCPDirectConnectionOptions describes the connection options for a direct connection.
type UCPDirectConnectionOptions struct {
	// Endpoint is the URL endpoint for the connection.
	Endpoint string `yaml:"endpoint"`
}

// NewConnectionFromUCPConfig creates a Connection for UCP endpoint.
func NewConnectionFromUCPConfig(option *UCPOptions, k8sConfig *rest.Config) (sdk.Connection, error) {
	if option.Kind == UCPConnectionKindDirect {
		if option.Direct == nil || option.Direct.Endpoint == "" {
			return nil, errors.New("the property .ucp.direct.endpoint is required when using a direct connection")
		}
		return sdk.NewDirectConnection(option.Direct.Endpoint)
	}
	return sdk.NewKubernetesConnectionFromConfig(k8sConfig)
}
