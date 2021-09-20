// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubernetes

import (
	bicepv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/bicep/v1alpha3"
	radiusv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/radius/v1alpha3"

	"github.com/Azure/azure-sdk-for-go/sdk/to"
	"github.com/Azure/radius/pkg/azure/radclient"
	"github.com/Azure/radius/pkg/kubernetes"
)

func ConvertK8sApplicationToARM(input radiusv1alpha3.Application) (*radclient.ApplicationResource, error) {
	result := radclient.ApplicationResource{}
	result.Name = to.StringPtr(input.Annotations[kubernetes.AnnotationsApplication])

	// There's nothing in properties for an application
	result.Properties = &radclient.ApplicationProperties{}

	return &result, nil
}

func ConvertK8sResourceToARM(input radiusv1alpha3.Resource) (*radclient.ComponentResource, error) {
	result := radclient.ComponentResource{}

	// TODO fix once we deal with client simplification and have RP changes
	return &result, nil
}

func ConvertK8sDeploymentToARM(input bicepv1alpha3.DeploymentTemplate) (*radclient.DeploymentResource, error) {
	result := radclient.DeploymentResource{}
	result.Properties = &radclient.DeploymentProperties{}

	// TODO remove once we deal with client simplification and have RP changes
	return &result, nil
}
