// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resourcesv1alpha3

// import (
// 	"encoding/json"
// 	"errors"
// )

// const kindProperty = "kind"
// const nameProperty = "name"
// const idProperty = "id"

// func (gt GenericTrait) MarshalJSON() ([]byte, error) {
// 	properties := map[string]interface{}{}
// 	for k, v := range gt.AdditionalProperties {
// 		properties[k] = v
// 	}

// 	properties[kindProperty] = gt.Kind
// 	return json.Marshal(properties)
// }

// func (gt *GenericTrait) UnmarshalJSON(b []byte) error {
// 	properties := map[string]interface{}{}
// 	err := json.Unmarshal(b, &properties)
// 	if err != nil {
// 		return err
// 	}

// 	obj, ok := properties[kindProperty]
// 	if !ok {
// 		return errors.New("the 'kind' property is required")
// 	}

// 	kind, ok := obj.(string)
// 	if !ok {
// 		return errors.New("the 'kind' property must be a string")
// 	}

// 	delete(properties, kindProperty)

// 	gt.Kind = kind
// 	gt.AdditionalProperties = properties
// 	return nil
// }
