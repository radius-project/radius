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
	// SecretValueEncodingRaw is the raw encoding type of value.
	SecretValueEncodingNone SecretValueEncoding = ""
	// SecretValueEncodingRaw is the raw encoding type of value.
	SecretValueEncodingRaw SecretValueEncoding = "raw"
	// SecretValueEncodingBase64 is the base64 encoding type of value.
	SecretValueEncodingBase64 SecretValueEncoding = "base64"
)

// SecretType represents the type of a secret.
type SecretType string

const (
	// SecretTypeNone is the undefined type.
	SecretTypeNone SecretType = ""
	// SecretTypeGeneric is the generic secret type.
	SecretTypeGeneric SecretType = "generic"
	// SecretTypeCert is the certificate secret type.
	SecretTypeCert SecretType = "certificate"
)

// SecretStore represents secret store resource.
type SecretStore struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties *SecretStoreProperties `json:"properties"`
}

// ResourceTypeName returns the resource type name of the resource.
func (s *SecretStore) ResourceTypeName() string {
	return "Applications.Core/secretStores"
}

// ApplyDeploymentOutput applies the properties changes based on the deployment output.
func (s *SecretStore) ApplyDeploymentOutput(do rpv1.DeploymentOutput) error {
	s.Properties.Status.OutputResources = do.DeployedOutputResources
	return nil
}

// OutputResources returns the output resources array.
func (s *SecretStore) OutputResources() []rpv1.OutputResource {
	return s.Properties.Status.OutputResources
}

// ResourceMetadata returns the application resource metadata.
func (s *SecretStore) ResourceMetadata() *rpv1.BasicResourceProperties {
	return &s.Properties.BasicResourceProperties
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
	// Encoding is the encoding type of Value.
	Encoding SecretValueEncoding `json:"encoding,omitempty"`
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

// SecretStoreListSecrets represents listSecret response.
type SecretStoreListSecrets struct {
	// Type is the type of the data.
	Type SecretType `json:"type,omitempty"`

	// Data is the data of the secret store.
	Data map[string]*SecretStoreDataValue `json:"data,omitempty"`
}

// ResourceTypeName returns the resource type name of the resource.
func (s *SecretStoreListSecrets) ResourceTypeName() string {
	return "Applications.Core/secretStores"
}

// ApplyDeploymentOutput applies the properties changes based on the deployment output.
func (s *SecretStoreListSecrets) ApplyDeploymentOutput(do rpv1.DeploymentOutput) error {
	return nil
}

// OutputResources returns the output resources array.
func (s *SecretStoreListSecrets) OutputResources() []rpv1.OutputResource {
	return nil
}

// ResourceMetadata returns SecretStoreListSecrets resource metadata.
func (s *SecretStoreListSecrets) ResourceMetadata() *rpv1.BasicResourceProperties {
	return nil
}
