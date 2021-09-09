// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package converters

import (
	"encoding/json"
	"errors"

	"github.com/Azure/radius/pkg/cli/armtemplate"
	radiusv1alpha1 "github.com/Azure/radius/pkg/kubernetes/api/radius/v1alpha1"
	"github.com/Azure/radius/pkg/model/resourcesv1alpha3"
	"k8s.io/apimachinery/pkg/conversion"
)

func ConvertComponentToInternal(a interface{}, b interface{}, scope conversion.Scope) error {
	original := a.(*radiusv1alpha1.Resource)
	result := b.(*resourcesv1alpha3.GenericResource)
	result.Name = original.Name
	result.Kind = original.Kind

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

	result.ID = armResource.ID

	if armResource.Body != nil {
		properties, ok := armResource.Body["properties"]
		if ok {
			data, err := json.Marshal(properties)
			if err != nil {
				return err
			}

			err = json.Unmarshal(data, &result.AdditionalProperties)
			if err != nil {
				return err
			}
		}
	}

	// if armResource != nil {
	// 	bytes, err := template.MarshalJSON()
	// 	if err != nil {
	// 		return err
	// 	}
	// 	err = json.Unmarshal(bytes, &result)
	// 	if err != nil {
	// 		return err
	// 	}
	// }

	// container
	//   image
	//   ports
	//     name
	//       containerPort
	//       provides

	// connections
	//   name
	//     Kind
	//     source

	return nil
}
