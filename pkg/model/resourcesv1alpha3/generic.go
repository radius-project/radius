// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resourcesv1alpha3

// GenericComponent represents a binding used by an Radius Component.
// type GenericResource struct {
// 	Name                 string                 `json:"name"`
// 	Kind                 string                 `json:"kind"`
// 	ID                   string                 `json:"id"`
// 	AdditionalProperties map[string]interface{} `json:"additionalProperties"`
// }

// // GenericDependency represents a binding used by an Radius Component.
// type GenericConnection struct {
// 	Kind   string `json:"kind"`
// 	Source string `json:"source"`
// }

// // GenericTrait represents a trait for an Radius component.
// type GenericTrait struct {
// 	Kind                 string
// 	AdditionalProperties map[string]interface{}

// 	// GenericTrait has custom marshaling code
// }

// func (generic GenericResource) As(kind string, specific interface{}) (bool, error) {
// 	if generic.Kind != kind {
// 		return false, nil
// 	}

// 	body := generic

// 	bytes, err := json.Marshal(body)
// 	if err != nil {
// 		return false, fmt.Errorf("failed to marshal generic component value: %w", err)
// 	}

// 	err = json.Unmarshal(bytes, specific)
// 	if err != nil {
// 		return false, fmt.Errorf("failed to unmarshal as value of type %T: %w", specific, err)
// 	}

// 	return true, nil
// }

// func (generic GenericResource) AsRequired(kind string, specific interface{}) error {
// 	match, err := generic.As(kind, specific)
// 	if err != nil {
// 		return err
// 	}

// 	if !match {
// 		return fmt.Errorf("the component was expected to have kind '%s', instead it is '%s'", kind, generic.Kind)
// 	}

// 	return nil
// }

// func (generic GenericResource) FindTrait(kind string, trait interface{}) (bool, error) {
// 	traits := generic.AdditionalProperties["traits"]
// 	if traits == nil {
// 		return false, nil
// 	}
// 	for _, t := range traits.([]GenericTrait) {
// 		if kind == t.Kind {
// 			return t.As(kind, trait)
// 		}
// 	}

// 	return false, nil
// }

// func (generic GenericTrait) As(kind string, specific interface{}) (bool, error) {
// 	if generic.Kind != kind {
// 		return false, nil
// 	}

// 	bytes, err := json.Marshal(generic)
// 	if err != nil {
// 		return false, fmt.Errorf("failed to marshal generic trait value: %w", err)
// 	}

// 	err = json.Unmarshal(bytes, specific)
// 	if err != nil {
// 		return false, fmt.Errorf("failed to unmarshal JSON as value of type %T: %w", specific, err)
// 	}

// 	return true, nil
// }
