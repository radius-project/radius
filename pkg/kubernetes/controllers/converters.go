// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controllers

import (
	"encoding/json"

	radiusv1alpha1 "github.com/Azure/radius/pkg/kubernetes/api/v1alpha1"
	"github.com/Azure/radius/pkg/radrp/components"
	"k8s.io/apimachinery/pkg/conversion"
)

func ConvertComponentToInternal(a interface{}, b interface{}, scope conversion.Scope) error {
	original := a.(*radiusv1alpha1.Component)
	result := b.(*components.GenericComponent)
	result.Name = original.Annotations["radius.dev/components"]
	result.Kind = original.Spec.Kind

	if original.Spec.Config != nil {
		b, err := original.Spec.Config.MarshalJSON()
		if err != nil {
			return err
		}

		result.Config = map[string]interface{}{}
		err = json.Unmarshal(b, &result.Config)
		if err != nil {
			return err
		}
	}

	if original.Spec.Run != nil {
		b, err := original.Spec.Run.MarshalJSON()
		if err != nil {
			return err
		}

		result.Run = map[string]interface{}{}
		err = json.Unmarshal(b, &result.Run)
		if err != nil {
			return err
		}
	}

	result.Bindings = map[string]components.GenericBinding{}

	j, err := original.Spec.Bindings.MarshalJSON()
	if err != nil {
		return err
	}
	err = json.Unmarshal(j, &result.Bindings)
	if err != nil {
		return err
	}

	if original.Spec.Uses != nil {
		for _, raw := range *original.Spec.Uses {
			b, err := raw.MarshalJSON()
			if err != nil {
				return err
			}

			dependency := components.GenericDependency{}
			err = json.Unmarshal(b, &dependency)
			if err != nil {
				return err
			}

			result.Uses = append(result.Uses, dependency)
		}
	}

	if original.Spec.Traits != nil {
		for _, raw := range *original.Spec.Traits {
			b, err := raw.MarshalJSON()
			if err != nil {
				return err
			}

			t := components.GenericTrait{}
			err = json.Unmarshal(b, &t)
			if err != nil {
				return err
			}

			result.Traits = append(result.Traits, t)
		}
	}

	return nil
}
