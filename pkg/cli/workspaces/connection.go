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
)

const KindKubernetes string = "kubernetes"

// MakeFallbackWorkspace creates an un-named workspace that will use the current KubeContext.
// This is is used in fallback cases where the user has no config.
func MakeFallbackWorkspace() *Workspace {
	return &Workspace{
		Source: SourceFallback,
		Connection: map[string]any{
			"kind":    KindKubernetes,
			"context": "", // Default Kubernetes context
		},
	}
}

type Connection interface {
	fmt.Stringer
	GetKind() string
}

// FmtConnection can safely format connection info for display to users.
func (ws Workspace) FmtConnection() string {
	c, err := ws.Connect()
	if err != nil {
		return fmt.Sprintf("err: %s", err)
	}

	return c.String()
}

func (ws Workspace) Connect() (Connection, error) {
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
		connection := &KubernetesConnection{}
		config := &mapstructure.DecoderConfig{ErrorUnused: true, Result: connection}
		decoder, err := mapstructure.NewDecoder(config)
		if err != nil {
			return nil, err
		}
		err = decoder.Decode(ws.Connection)
		if err != nil {
			return nil, err
		}

		return connection, nil

	default:
		return nil, fmt.Errorf("unsupported connection kind '%s'", kind)
	}

}

func (ws Workspace) ConnectionEquals(other Connection) bool {
	switch other.GetKind() {
	case KindKubernetes:
		kc, ok := other.(*KubernetesConnection)
		if !ok {
			return false
		}

		return ws.Connection["kind"] == KindKubernetes && ws.IsSameKubernetesContext(kc.Context)
	default:
		return false
	}
}

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

func (ws Workspace) IsSameKubernetesContext(kubeContext string) bool {
	return ws.Connection["context"] == kubeContext
}

var _ Connection = (*KubernetesConnection)(nil)

type KubernetesConnection struct {
	// Kind specifies the kind of connection. For KubernetesConnection this is always 'kubernetes'.
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

func (c *KubernetesConnection) String() string {
	return fmt.Sprintf("Kubernetes (context=%s)", c.Context)
}

func (c *KubernetesConnection) GetKind() string {
	return KindKubernetes
}

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

	config, err := kubernetes.GetConfig(c.Context)
	if err != nil {
		return nil, err
	}

	return sdk.NewKubernetesConnectionFromConfig(config)
}
