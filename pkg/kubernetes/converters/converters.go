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
	"k8s.io/apimachinery/pkg/conversion"
)

func ConvertComponentToInternal(a interface{}, b interface{}, scope conversion.Scope) error {
	original := a.(*radiusv1alpha3.Resource)
	result := b.(*renderers.RendererResource)
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
