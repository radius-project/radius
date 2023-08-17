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

package workspaces

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/mitchellh/mapstructure"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/pkg/sdk"
)

const KindKubernetes string = "kubernetes"

// MakeFallbackWorkspace creates an un-named workspace that will use the current KubeContext.
// This is is used in fallback cases where the user has no config.
//

// MakeFallbackWorkspace() creates a Workspace struct with a SourceFallback source and a Kubernetes connection with an
// empty context. It returns a pointer to the Workspace struct.
func MakeFallbackWorkspace() *Workspace {
	return &Workspace{
		Source: SourceFallback,
		Connection: map[string]any{
			"kind":    KindKubernetes,
			"context": "", // Default Kubernetes context
		},
	}
}

type ConnectionConfig interface {
	fmt.Stringer
	GetKind() string
	Connect() (sdk.Connection, error)
}

// FmtConnection can safely format connection info for display to users.
func (ws Workspace) FmtConnection() string {
	c, err := ws.ConnectionConfig()
	if err != nil {
		return fmt.Sprintf("err: %s", err)
	}

	return c.String()
}

// ConnectionConfig() checks the "kind" field of the workspace's Connection object and returns a
// ConnectionConfig object based on the kind, or an error if the kind is unsupported.
func (ws Workspace) ConnectionConfig() (ConnectionConfig, error) {
	obj, ok := ws.Connection["kind"]
	if !ok {
		return nil, fmt.Errorf("workspace is missing required field '$.connection.kind'")
	}

	kind, ok := obj.(string)
	if !ok {
		return nil, fmt.Errorf("workspace field '$.connection.kind' must be a string")
	}

	switch kind {
	case KindKubernetes:
		config := &KubernetesConnectionConfig{}
		decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{ErrorUnused: true, Result: config})
		if err != nil {
			return nil, err
		}

		err = decoder.Decode(ws.Connection)
		if err != nil {
			return nil, err
		}

		return config, nil
	default:
		return nil, fmt.Errorf("unsupported connection kind '%s'", kind)
	}
}

// Connect attempts to create a connection to the workspace using the connection configuration and returns the
// connection and an error if one occurs.
func (ws Workspace) Connect() (sdk.Connection, error) {
	connectionConfig, err := ws.ConnectionConfig()
	if err != nil {
		return nil, err
	}

	return connectionConfig.Connect()
}

// ConnectionConfigEquals() checks if the given ConnectionConfig is of type Kubernetes and if the Kubernetes
// context is the same as the one stored in the Workspace, and returns a boolean value accordingly.
func (ws Workspace) ConnectionConfigEquals(other ConnectionConfig) bool {
	switch other.GetKind() {
	case KindKubernetes:
		kc, ok := other.(*KubernetesConnectionConfig)
		if !ok {
			return false
		}

		return ws.Connection["kind"] == KindKubernetes && ws.IsSameKubernetesContext(kc.Context)
	default:
		return false
	}
}

// KubernetesContext checks if the workspace connection is of type Kubernetes and returns the context string if it exists,
// otherwise it returns an empty string and false.
func (ws Workspace) KubernetesContext() (string, bool) {
	if ws.Connection["kind"] != KindKubernetes {
		return "", false
	}

	obj, ok := ws.Connection["context"]
	if !ok {
		return "", false
	}

	str, ok := obj.(string)
	if !ok {
		return "", false
	}

	return str, true
}

// IsSameKubernetesContext checks if the "context" field of the "Connection" map of the "Workspace" struct is equal to
// the given "kubeContext" string and returns a boolean value accordingly.
func (ws Workspace) IsSameKubernetesContext(kubeContext string) bool {
	return ws.Connection["context"] == kubeContext
}

var _ ConnectionConfig = (*KubernetesConnectionConfig)(nil)

type KubernetesConnectionConfig struct {
	// Kind specifies the kind of connection. For KubernetesConnectionConfig this is always 'kubernetes'.
	Kind string `json:"kind" mapstructure:"kind" yaml:"kind"`

	// Context is the kubernetes kubeconfig context used to connect. The empty string is allowed as it
	// maps to the current kubeconfig context.
	Context string `json:"context" mapstructure:"context" yaml:"context"`

	// Overrides describes local overrides for testing purposes. This field is optional.
	Overrides KubernetesConnectionOverrides `json:"overrides,omitempty" mapstructure:"overrides" yaml:"overrides,omitempty"`
}

type KubernetesConnectionOverrides struct {
	// UCP describes an override for testing UCP. this field is optional.
	UCP string `json:"ucp" mapstructure:"ucp" yaml:"ucp"`
}

// String() returns a string that describes the Kubernetes connection configuration.
func (c *KubernetesConnectionConfig) String() string {
	if c.Overrides.UCP == "" {
		return fmt.Sprintf("Kubernetes (context=%s)", c.Context)
	}

	return fmt.Sprintf("Kubernetes (context=%s, ucp=%s)", c.Context, c.Overrides.UCP)
}

// GetKind() returns the string "KindKubernetes" for a KubernetesConnectionConfig object.
func (c *KubernetesConnectionConfig) GetKind() string {
	return KindKubernetes
}

// Connect() checks if a URL is provided in the Overrides field, and if so, creates a direct connection to the URL. If no URL
// is provided, it creates a connection to Kubernetes using the provided context. If an error occurs, an error is returned.
func (c *KubernetesConnectionConfig) Connect() (sdk.Connection, error) {
	if c.Overrides.UCP != "" {
		strURL := strings.TrimSuffix(c.Overrides.UCP, "/")
		strURL = strURL + "/apis/api.ucp.dev/v1alpha3"
		_, err := url.ParseRequestURI(strURL)
		if err != nil {
			return nil, err
		}
		return sdk.NewDirectConnection(strURL)
	}

	config, err := kubernetes.NewCLIClientConfig(c.Context)
	if err != nil {
		return nil, err
	}

	return sdk.NewKubernetesConnectionFromConfig(config)
}
