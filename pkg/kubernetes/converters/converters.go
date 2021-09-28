// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package converters

import (
	"encoding/json"
	"errors"

	"github.com/Azure/radius/pkg/cli/armtemplate"
	radiusv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/radius/v1alpha3"
	"github.com/Azure/radius/pkg/renderers"
)

// Since the resource will be processed by an ARM template we need to convert it to an ARM-like representation.
func ConvertToARMResource(original *radiusv1alpha3.Resource, body map[string]interface{}) error {
	properties, ok := body["properties"].(map[string]interface{})
	if !ok {
		properties = map[string]interface{}{}
		body["properties"] = properties
	}

	// Using the user-provided definition as a 'base' merge in the computed properties
	if original.Status.ComputedValues != nil {
		computedValues := map[string]renderers.ComputedValueReference{}

		err := json.Unmarshal(original.Status.ComputedValues.Raw, &computedValues)
		if err != nil {
			return err
		}

		for key, value := range computedValues {
			properties[key] = value.Value
		}
	}

	return nil
}

func ConvertToRenderResource(original *radiusv1alpha3.Resource, result *renderers.RendererResource) error {
	result.ResourceName = original.Name
	result.ResourceType = original.Kind
	result.ApplicationName = original.Spec.Application

	template := original.Spec.Template

	// Get arm template from template part
	if template == nil {
		return errors.New("must have template as part of CRD")
	}

	armResource := &armtemplate.Resource{}
	err := json.Unmarshal(template.Raw, armResource)

	if err != nil {
		return err
	}

	if armResource.Body != nil {
		properties, ok := armResource.Body["properties"]
		if ok {
			data, err := json.Marshal(properties)
			if err != nil {
				return err
			}

			err = json.Unmarshal(data, &result.Definition)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
