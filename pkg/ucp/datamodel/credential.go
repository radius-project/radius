// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
)

// Credential represents UCP Credential.
type Credential struct {
	v1.TrackedResource

	Properties *CredentialResourceProperties `json:"properties,omitempty"`
}

func (c *Credential) ResourceTypeName() string {
	return c.Type
}

// Credential Properties represents UCP Credential Properties.
type CredentialResourceProperties struct {
	Kind            string                       `json:"kind,omitempty"`
	AzureCredential *AzureCredentialProperties   `json:"azureCredential,omitempty"`
	AWSCredential   *AWSCredentialProperties     `json:"awsCredential,omitempty"`
	Storage         *CredentialStorageProperties `json:"storage,omitempty"`
}

type AzureCredentialProperties struct {
	TenantID *string `json:"tenantId,omitempty"`
	ClientID *string `json:"clientId,omitempty"`
}

type AWSCredentialProperties struct {
	AccessKeyID     *string `json:"accessKeyId,omitempty"`
	SecretAccessKey *string `json:"secretAccessKey,omitempty"`
}

type CredentialStorageKind string

type CredentialStorageProperties struct {
	Kind               *CredentialStorageKind               `json:"kind,omitempty"`
	InternalCredential *InternalCredentialStorageProperties `json:"internalCredential,omitempty"`
}

type InternalCredentialStorageProperties struct {
	SecretName *string `json:"secretName"`
}
