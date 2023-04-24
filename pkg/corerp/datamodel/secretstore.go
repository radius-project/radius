// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
)

// SecretValueEncoding is the encoding type.
type SecretValueEncoding string

const (
	// SecretNoneEnconding is the undefined encoding type of value.
	SecretValueEncodingNone SecretValueEncoding = ""
	// SecretValueEncodingRaw is the raw encoding type of value.
	SecretValueEncodingRaw SecretValueEncoding = "raw"
	// SecretValueEncodingBase64 is the base64 encoding type of value.
	SecretValueEncodingBase64 SecretValueEncoding = "base64"
)

// SecretType represents the type of a secret.
type SecretType string

const (
	// SecretTypeGeneric is the generic secret type.
	SecretTypeGeneric SecretType = "generic"
	// SecretTypeCert is the certificate secret type.
	SecretTypeCert SecretType = "certificate"
)

// SecretStore represents Application environment resource.
type SecretStore struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties *SecretStoreProperties `json:"properties"`
}

// ResourceTypeName returns the resource type name of the resource.
func (e *SecretStore) ResourceTypeName() string {
	return "Applications.Core/secretStores"
}

// ApplyDeploymentOutput applies the properties changes based on the deployment output.
func (h *SecretStore) ApplyDeploymentOutput(do rpv1.DeploymentOutput) error {
	h.Properties.Status.OutputResources = do.DeployedOutputResources
	return nil
}

// OutputResources returns the output resources array.
func (h *SecretStore) OutputResources() []rpv1.OutputResource {
	return h.Properties.Status.OutputResources
}

// ResourceMetadata returns the application resource metadata.
func (h *SecretStore) ResourceMetadata() *rpv1.BasicResourceProperties {
	return &h.Properties.BasicResourceProperties
}

// SecretStoreProperties represents the properties of SecretStore.
type SecretStoreProperties struct {
	rpv1.BasicResourceProperties

	// Type is the type of the data.
	Type SecretType `json:"type,omitempty"`

	// Data is the data of the secret store.
	Data map[string]*SecretStoreDataValue `json:"data,omitempty"`

	// Resource is the resource id of an external secret store.
	Resource string `json:"resource,omitempty"`
}

// SecretStoreDataValue represents the value of the secret store data.
type SecretStoreDataValue struct {
	// Value is the value of the secret store data.
	Value *string `json:"value,omitempty"`
	// ValueFrom is the value from of the secret store data.
	ValueFrom *SecretStoreDataValueFrom `json:"valueFrom,omitempty"`
}

// SecretStoreDataValueFrom represents the secret reference in the secret store.
type SecretStoreDataValueFrom struct {
	// Name is the name of the secret.
	Name string `json:"name,omitempty"`
	// Version is the version of the secret.
	Version string `json:"version,omitempty"`
}
