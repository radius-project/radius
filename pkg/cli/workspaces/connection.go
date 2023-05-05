// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

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
// # Function Explanation
// 
//	MakeFallbackWorkspace creates a Workspace object with a Source of SourceFallback and a Connection of KindKubernetes with
//	 an empty context, providing a fallback option for callers of this function. If the callers do not provide a Workspace 
//	object, this function will provide a default one.
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
//
// # Function Explanation
// 
//	Workspace.FmtConnection() creates a connection configuration from the workspace and returns a string representation of 
//	it, or an error message if an error occurs.
func (ws Workspace) FmtConnection() string {
	c, err := ws.ConnectionConfig()
	if err != nil {
		return fmt.Sprintf("err: %s", err)
	}

	return c.String()
}

// # Function Explanation
// 
//	Workspace.ConnectionConfig() takes in a Workspace object and returns a ConnectionConfig object or an error. It checks if
//	 the Workspace object has a "kind" field in its Connection field, and if it does, it checks the value of the "kind" 
//	field to determine which type of ConnectionConfig object to return. If the "kind" field is not present, it returns an 
//	error. If the "kind" field is present but is not a string, it returns an error. If the "kind" field is present and is a 
//	string, it uses the value of the "kind" field to determine which type of ConnectionConfig object to return. If an error 
//	occurs during the decoding process, it returns an error.
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

// # Function Explanation
// 
//	Workspace.Connect() establishes a connection to the workspace using the connection configuration and returns the 
//	connection or an error if one occurs.
func (ws Workspace) Connect() (sdk.Connection, error) {
	connectionConfig, err := ws.ConnectionConfig()
	if err != nil {
		return nil, err
	}

	return connectionConfig.Connect()
}

// # Function Explanation
// 
//	Workspace.ConnectionConfigEquals compares the given ConnectionConfig to the Workspace's ConnectionConfig and returns 
//	true if they are the same. If the ConnectionConfig is not of type Kubernetes, it returns false. If it is of type 
//	Kubernetes, it checks if the Contexts are the same and returns true if they are. If the ConnectionConfig is not of type 
//	Kubernetes, it returns false.
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

// # Function Explanation
// 
//	Workspace.KubernetesContext() checks if the Connection field of the Workspace object is of type KindKubernetes and, if 
//	so, returns the context string from the Connection field. If the Connection field is not of type KindKubernetes or the 
//	context string is not present, it returns an empty string and false.
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

// # Function Explanation
// 
//	"IsSameKubernetesContext" checks if the Kubernetes context stored in the "Connection" field of the Workspace struct is 
//	the same as the one provided as an argument. If it is, it returns true, otherwise false. If an error occurs, it will be 
//	returned to the caller.
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

// # Function Explanation
// 
//	The KubernetesConnectionConfig.String() function returns a string representation of the KubernetesConnectionConfig 
//	object, including the context. If an error occurs, it is returned to the caller.
func (c *KubernetesConnectionConfig) String() string {
	return fmt.Sprintf("Kubernetes (context=%s)", c.Context)
}

// # Function Explanation
// 
//	The GetKind() function of the KubernetesConnectionConfig struct returns the string "Kubernetes" if no errors occur. If 
//	an error does occur, it will be returned to the caller.
func (c *KubernetesConnectionConfig) GetKind() string {
	return KindKubernetes
}

// # Function Explanation
// 
//	The Connect function in KubernetesConnectionConfig checks if the UCP URL is provided in the Overrides field and if so, 
//	creates a DirectConnection with the URL. If not, it creates a KubernetesConnectionFromConfig using the provided Context.
//	 If any errors occur during the process, they are returned to the caller.
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
