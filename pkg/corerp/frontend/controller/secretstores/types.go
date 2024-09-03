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

package secretstores

const (
	// ResourceTypeName is the resource type name for secret stores.
	ResourceTypeName = "Applications.Core/secretStores"

	// UsernameKey is a required key in a secret store when SecretType is Basic Authentication.
	UsernameKey = "username"

	// PasswordKey is a required key in a secret store when SecretType is Basic Authentication.
	PasswordKey = "password"

	// ClientIdKey is a required key in a secret store when SecretType is Azure Workload Identity.
	ClientIdKey = "clientId"

	// TenantIdKey is a required key in a secret store when SecretType is Azure workload Identity.
	TenantIdKey = "tenantId"

	// RoleARNKey is a required key in a  secret store when SecretType is AWS IRSA.
	RoleARNKey = "roleARN"
)
