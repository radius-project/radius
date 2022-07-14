// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

// workspaces contains functionality for using the workspace concept of the CLI to connect and interact
// with the remote endpoints that are described by the workspace concept
// (Radius control plane, environment, et al).
package workspaces

import (
	"fmt"

	"github.com/mitchellh/mapstructure"
)

const KindKubernetes string = "kubernetes"

type Connection interface {
	GetKind() string
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

		return ws.Connection["kind"] == KindKubernetes && ws.Connection["context"] == kc.Context
	default:
		return false
	}
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

func (c *KubernetesConnection) GetKind() string {
	return KindKubernetes
}
