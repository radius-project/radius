// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package radlogger

// Field names for structured logging
const (
	LogFieldTimeStamp          = "timestamp"
	LogFieldAppName            = "application"
	LogFieldAppID              = "applicationID"
	LogFieldComponentName      = "component"
	LogFieldSubscriptionID     = "subscriptionId"
	LogFieldResourceGroup      = "resourceGroup"
	LogFieldAction             = "action"
	LogFieldComponentKind      = "componentKind"
	LogFieldDeploymentName     = "deployment"
	LogFieldErrors             = "errors"
	LogFieldResourceType       = "resourceType"
	LogFieldResourceProperties = "resourceProperties"
)

const ContextLoggerField = "logger"
