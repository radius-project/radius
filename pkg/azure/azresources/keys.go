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

package azresources

const (
	// SubscriptionIDKey is the route parameter key for the subscription ID segment of the URL.
	SubscriptionIDKey = "subscriptionId"

	// ResourceGroupKey is the route parameter key for the resource group segment of the URL.
	ResourceGroupKey = "resourceGroup"

	// ResourceProviderKey is the route parameter key for the resource provider name segment of the URL.
	ResourceProviderKey = "resourceProvider"

	// ResourceNameKey is the route parameter key for the resource name segment of the URL.
	ResourceNameKey = "resourceName"

	// ApplicationNameKey is the route parameter key for the application name segment of the URL.
	ApplicationNameKey = "applicationName"

	// ResourceTypeKey is the route parameter key for the resource type (child resource of application) segment of the URL.
	ResourceTypeKey = "resourceType"

	// OperationIDKey is the route parameter key for the operation id segment of the URL.
	OperationIDKey = "operationId"
)
