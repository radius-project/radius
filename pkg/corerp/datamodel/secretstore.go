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

package datamodel

import (
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
)

const SecretStoreResourceType = "Applications.Core/secretStores"

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

	// TODO: remove this from CoreRP
	LinkMetadata
	// Properties is the properties of the resource.
	Properties *SecretStoreProperties `json:"properties"`
}

// ResourceTypeName returns the resource type name of the SecretStore instance.
func (s *SecretStore) ResourceTypeName() string {
	return "Applications.Core/secretStores"
}

// ApplyDeploymentOutput updates the status of the SecretStore instance with the output resources from the DeploymentOutput
// object and returns no error.
func (s *SecretStore) ApplyDeploymentOutput(do rpv1.DeploymentOutput) error {
	s.Properties.Status.OutputResources = do.DeployedOutputResources
	return nil
}

// OutputResources returns the OutputResources from the SecretStore's Properties.
func (s *SecretStore) OutputResources() []rpv1.OutputResource {
	return s.Properties.Status.OutputResources
}

// ResourceMetadata returns the BasicResourceProperties of the SecretStore instance.
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

// ResourceTypeName returns the resource type name of the SecretStoreListSecrets struct.
func (s *SecretStoreListSecrets) ResourceTypeName() string {
	return "Applications.Core/secretStores"
}

// ApplyDeploymentOutput applies the deployment output to the SecretStoreListSecrets instance and returns no error.
func (s *SecretStoreListSecrets) ApplyDeploymentOutput(do rpv1.DeploymentOutput) error {
	return nil
}

// OutputResources returns nil for SecretStoreListSecrets.
func (s *SecretStoreListSecrets) OutputResources() []rpv1.OutputResource {
	return nil
}

// ResourceMetadata returns nil for SecretStoreListSecrets.
func (s *SecretStoreListSecrets) ResourceMetadata() *rpv1.BasicResourceProperties {
	return nil
}
