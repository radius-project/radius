// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"

	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
)

const (
	// Common K8s Keys
	KubernetesAPIVersionKey = "kubernetesapiversion"
	KubernetesKindKey       = "kuberneteskind"
	KubernetesNamespaceKey  = "kubernetesnamespace"
	KubernetesNameKey       = "kubernetesname"
	ResourceName            = "resourcename"
)

const (
	// Common Keys
	APIVersionKey   = "kubernetesapiversion"
	KindKey         = "kuberneteskind"
	NamespaceKey    = "kubernetesnamespace"
	NameKey         = "kubernetesname"
	ResourceNameKey = "resourcename"
)

// PutOptions represents the options for ResourceHandler.Put.
type PutOptions struct {
	// Resource represents the rendered resource.
	Resource *rpv1.OutputResource

	// DependencyProperties is a map of output resource localID to resource properties populated during deployment in the resource handler
	DependencyProperties map[string]map[string]string
}

// DeleteOptions represents the options for ResourceHandler.Delete.
type DeleteOptions struct {
	// Resource represents the rendered resource.
	Resource *rpv1.OutputResource
}

// ResourceHandler interface defines the methods that every output resource will implement
//
//go:generate mockgen -destination=./mock_resource_handler.go -package=handlers -self_package github.com/project-radius/radius/pkg/corerp/handlers github.com/project-radius/radius/pkg/corerp/handlers ResourceHandler
type ResourceHandler interface {
	// Put deploys the rendered output resource and returns and populates the properties during deployment,
	// which can be used by the next resource handlers.
	Put(ctx context.Context, options *PutOptions) (map[string]string, error)

	// Delete deletes the rendered output resource.
	Delete(ctx context.Context, options *DeleteOptions) error
}
