// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"

	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/resourcemodel"
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

// ResourceHandler interface defines the methods that every output resource will implement
//go:generate mockgen -destination=./mock_resource_handler.go -package=handlers -self_package github.com/project-radius/radius/pkg/corerp/handlers github.com/project-radius/radius/pkg/corerp/handlers ResourceHandler
type ResourceHandler interface {
	Put(ctx context.Context, resource *outputresource.OutputResource) error
	GetResourceIdentity(ctx context.Context, resource outputresource.OutputResource) (resourcemodel.ResourceIdentity, error)
	GetResourceNativeIdentityKeyProperties(ctx context.Context, resource outputresource.OutputResource) (map[string]string, error)
	Delete(ctx context.Context, resource outputresource.OutputResource) error
}
