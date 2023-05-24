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

package clients

type Providers struct {
	// Azure provider information. This field is optional.
	Azure *AzureProvider
	// AWS provider information. This field is optional.
	AWS *AWSProvider
	// Radius provider information.
	Radius *RadiusProvider
}

type AzureProvider struct {
	// Scope is the target level for deploying the Azure resources.
	Scope string
}

type AWSProvider struct {
	// Scope is the target level for deploying the AWS resources.
	Scope string
}

type RadiusProvider struct {
	// Currently, we must provide an environment ID for deploying applications.
	EnvironmentID string
	// ApplicationID is the ID of the application to be deployed. This is optional.
	ApplicationID string
}
