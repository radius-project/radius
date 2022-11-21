// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/rp"
)

var _ conv.DataModelInterface = (*Application)(nil)

// ApplicationInternalMetadata represents the internal metadata for application resource, which hold any metadata used internally.
type ApplicationInternalMetadata struct {
	// KubernetesNamespace represents the application level kuberentes namespace name.
	KubernetesNamespace string `json:"kubernetesNamespace,omitempty"`
}

// Application represents Application resource.
type Application struct {
	v1.BaseResource

	// AppInternal represents Application internal metadata.
	AppInternal *ApplicationInternalMetadata `json:"appInternal,omitempty"`

	// Properties is the properties of the resource.
	Properties ApplicationProperties `json:"properties"`
}

func (e *Application) ResourceTypeName() string {
	return "Applications.Core/applications"
}

// ApplicationProperties represents the properties of Application.
type ApplicationProperties struct {
	rp.BasicResourceProperties
	Extensions []Extension `json:"extensions,omitempty"`
}

// FindExtension finds the right extension.
func (a *ApplicationProperties) FindExtension(kind ExtensionKind) *Extension {
	for _, ext := range a.Extensions {
		if ext.Kind == kind {
			return &ext
		}
	}
	return nil
}
