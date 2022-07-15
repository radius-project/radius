// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package providers

import "github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"

// Providers supported by Radius
// The RP will be able to support a resource only if the corresponding provider is configured with the RP
const (
	ProviderAzure = "azure"
	// This is a special case for support AAD Pod Identity which is not an ARM resource but a modification of an AKS Cluster
	ProviderAzureKubernetesService = "aks"
	ProviderKubernetes             = "kubernetes"
)

type Deployment struct {
	// Location - The location to store the deployment data.
	Location *string `json:"location,omitempty"`
	// Properties - The deployment properties.
	Properties *DeploymentProperties `json:"properties,omitempty"`
	// Tags - Deployment tags
	Tags map[string]*string `json:"tags"`
}

// DeploymentProperties deployment properties.
type DeploymentProperties struct {
	// Template - The template content. You use this element when you want to pass the template syntax directly in the request rather than link to an existing template. It can be a JObject or well-formed JSON string. Use either the templateLink property or the template property, but not both.
	Template interface{} `json:"template,omitempty"`
	// TemplateLink - The URI of the template. Use either the templateLink property or the template property, but not both.
	TemplateLink *resources.TemplateLink `json:"templateLink,omitempty"`
	//ProviderConfig specifies the scope for resources
	ProviderConfig interface{} `json:"providerconfig,omitempty"`
	// Parameters - Name and value pairs that define the deployment parameters for the template. You use this element when you want to provide the parameter values directly in the request rather than link to an existing parameter file. Use either the parametersLink property or the parameters property, but not both. It can be a JObject or a well formed JSON string.
	Parameters interface{} `json:"parameters,omitempty"`
	// ParametersLink - The URI of parameters file. You use this element to link to an existing parameters file. Use either the parametersLink property or the parameters property, but not both.
	ParametersLink *resources.ParametersLink `json:"parametersLink,omitempty"`
	// Mode - The mode that is used to deploy resources. This value can be either Incremental or Complete. In Incremental mode, resources are deployed without deleting existing resources that are not included in the template. In Complete mode, resources are deployed and existing resources in the resource group that are not included in the template are deleted. Be careful when using Complete mode as you may unintentionally delete resources. Possible values include: 'DeploymentModeIncremental', 'DeploymentModeComplete'
	Mode resources.DeploymentMode `json:"mode,omitempty"`
	// DebugSetting - The debug setting of the deployment.
	DebugSetting *resources.DebugSetting `json:"debugSetting,omitempty"`
	// OnErrorDeployment - The deployment on error behavior.
	OnErrorDeployment *resources.OnErrorDeployment `json:"onErrorDeployment,omitempty"`
	// ExpressionEvaluationOptions - Specifies whether template expressions are evaluated within the scope of the parent template or nested template. Only applicable to nested templates. If not specified, default value is outer.
	ExpressionEvaluationOptions *resources.ExpressionEvaluationOptions `json:"expressionEvaluationOptions,omitempty"`
}

type Value struct {
	Scope string `json:"scope,omitempty"`
}

type Radius struct {
	Type  string `json:"type,omitempty"`
	Value Value  `json:"value,omitempty"`
}

type Az struct {
	Type  string `json:"type,omitempty"`
	Value Value  `json:"value,omitempty"`
}

type Deployments struct {
	Type  string `json:"type,omitempty"`
	Value Value  `json:"value,omitempty"`
}

type ProviderConfig struct {
	Radius      *Radius      `json:"radius,omitempty"`
	Az          *Az          `json:"az,omitempty"`
	Deployments *Deployments `json:"deployments,omitempty"`
}
