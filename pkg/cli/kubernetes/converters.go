// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubernetes

import (
	"encoding/json"

	radiusv1alpha1 "github.com/Azure/radius/pkg/kubernetes/api/v1alpha1"

	"github.com/Azure/azure-sdk-for-go/sdk/to"
	"github.com/Azure/radius/pkg/kubernetes"
	"github.com/Azure/radius/pkg/radclient"
)

func ConvertK8sApplicationToARM(input radiusv1alpha1.Application) (*radclient.ApplicationResource, error) {
	result := radclient.ApplicationResource{}
	result.Name = to.StringPtr(input.Annotations[kubernetes.AnnotationsApplication])

	// There's nothing in properties for an application
	result.Properties = map[string]interface{}{}

	return &result, nil
}

func ConvertK8sComponentToARM(input radiusv1alpha1.Component) (*radclient.ComponentResource, error) {
	result := radclient.ComponentResource{}
	result.Name = to.StringPtr(input.Annotations[kubernetes.AnnotationsComponent])
	result.Kind = &input.Spec.Kind
	result.Properties = &radclient.ComponentProperties{}

	if input.Spec.Config != nil {
		bytes, err := input.Spec.Config.MarshalJSON()
		if err != nil {
			return nil, err
		}

		result.Properties.Config = map[string]interface{}{}
		err = json.Unmarshal(bytes, &result.Properties.Config)
		if err != nil {
			return nil, err
		}
	}

	if input.Spec.Run != nil {
		bytes, err := input.Spec.Run.MarshalJSON()
		if err != nil {
			return nil, err
		}

		result.Properties.Run = map[string]interface{}{}
		err = json.Unmarshal(bytes, &result.Properties.Run)
		if err != nil {
			return nil, err
		}
	}

	result.Properties.Bindings = map[string]interface{}{}

	bindingBytes, err := input.Spec.Bindings.MarshalJSON()
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(bindingBytes, &result.Properties.Bindings)
	if err != nil {
		return nil, err
	}

	if input.Spec.Uses != nil {
		for _, raw := range *input.Spec.Uses {
			bytes, err := raw.MarshalJSON()
			if err != nil {
				return nil, err
			}

			dependency := map[string]interface{}{}
			err = json.Unmarshal(bytes, &dependency)
			if err != nil {
				return nil, err
			}

			result.Properties.Uses = append(result.Properties.Uses, dependency)
		}
	}

	if input.Spec.Traits != nil {
		for _, raw := range *input.Spec.Traits {
			bytes, err := raw.MarshalJSON()
			if err != nil {
				return nil, err
			}
			t, err := radclient.UnmarshalComponentTraitClassification(json.RawMessage(bytes))
			if err != nil {
				return nil, err
			}
			result.Properties.Traits = append(result.Properties.Traits, t)
		}
	}

	return &result, nil
}

func ConvertK8sDeploymentToARM(input radiusv1alpha1.Deployment) (*radclient.DeploymentResource, error) {
	result := radclient.DeploymentResource{}
	result.Name = to.StringPtr(input.Annotations[kubernetes.AnnotationsDeployment])
	result.Properties = &radclient.DeploymentProperties{}

	for _, dc := range input.Spec.Components {
		converted := radclient.DeploymentPropertiesComponentsItem{
			ComponentName: to.StringPtr(dc.ComponentName),
		}
		result.Properties.Components = append(result.Properties.Components, &converted)
	}

	return &result, nil
}
