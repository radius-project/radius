// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package armtemplate

type DeploymentResource struct {
	Properties DeploymentResourceProperties `json:"properties"`
}

type DeploymentResourceProperties struct {
	Parameters map[string]map[string]interface{} `json:"parameters"`
	Template   DeploymentTemplate                `json:"template"`
}
