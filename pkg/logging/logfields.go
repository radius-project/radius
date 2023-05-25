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
package logging

const (
	AppCoreLoggerName string = "applications.core"
	AppLinkLoggerName string = "applications.link"

	ServiceName string = "rp"
)

// Field names for structured logging
const (
	LogFieldAction             = "action"
	LogFieldAppID              = "applicationId"
	LogFieldAppName            = "applicationName"
	LogFieldCorrelationID      = "correlationId"
	LogFieldDeploymentID       = "deploymentId"
	LogFieldDeploymentName     = "deploymentName"
	LogFieldKind               = "kind"
	LogFieldLocalID            = "localId"
	LogFieldNamespace          = "namespace"
	LogFieldOperationID        = "operationId"
	LogFieldOperationType      = "operationType"
	LogFieldDequeueCount       = "dequeueCount"
	LogFieldOperationStatus    = "operationStatus"
	LogFieldResourceGroup      = "resourceGroup"
	LogFieldResourceID         = "resourceId"
	LogFieldResourceName       = "resourceName"
	LogFieldResourceProperties = "resourceProperties"
	LogFieldResourceType       = "resourceType"
	LogFieldRPIdentifier       = "rpIdentifier"
	LogFieldScopeName          = "scopeName"
	LogFieldSubscriptionID     = "subscriptionId"
	LogFieldResourceKind       = "resourceKind"
	LogHTTPStatusCode          = "statusCode"
)
