/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package processors

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/recipes"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
)

const (
	kindConnectionValue  = "connection value"
	kindConnectionSecret = "connection secret"
)

// Validator provides validation support to be used with the data model of a resource type that supports recipes.
//
// The Validator can be used to:
//
// - Extract output resources from a `resource` field
// - Extract output resources from the recipe out
// - Extract connection values and connection secrets from the recipe output
// - Apply values and secrets from the recipe output to the resource data model.
type Validator struct {
	resourcesField *[]*linkrp.ResourceReference
	fields         []func(output *recipes.RecipeOutput) string
	computedFields []func(output *recipes.RecipeOutput) string

	// ConnectionValues stores the connection values extracted from the data model and recipe output.
	ConnectionValues map[string]any

	// ConnectionSecrets stores the connection secrets extracted from the data model and recipe output.
	ConnectionSecrets map[string]rpv1.SecretValueReference

	// OutputResources stores the output resources extracted from the data model and recipe output.
	OutputResources *[]rpv1.OutputResource
}

// NewValidator creates a new Validator. Use the parameters to pass in pointers to the corresponding fields on the resource
// data model.
func NewValidator(connectionValues *map[string]any, connectionSecrets *map[string]rpv1.SecretValueReference, outputResources *[]rpv1.OutputResource) *Validator {
	// Empty the computed data structures. This ensures that we don't accumulate data from previous validations.
	*connectionValues = map[string]any{}
	*connectionSecrets = map[string]rpv1.SecretValueReference{}
	*outputResources = []rpv1.OutputResource{}

	return &Validator{
		ConnectionValues:  *connectionValues,
		ConnectionSecrets: *connectionSecrets,
		OutputResources:   outputResources,
	}
}

// AddResourceField registers a field containing a resource ID with the validator.
func (v *Validator) AddResourcesField(ref *[]*linkrp.ResourceReference) {
	v.resourcesField = ref
}

// AddRequiredInt32Field registers a field containing a required int32 connection value. The zero value will be treated as an "unset" value.
func (v *Validator) AddRequiredInt32Field(name string, ref *int32) {
	v.fields = append(v.fields, bind(v, name, ref, true, false, "int32", convertToInt32, nil))
}

// AddOptionalInt32Field registers a field containing an optional int32 connection value. The zero value will be treated as an "unset" value.
func (v *Validator) AddOptionalInt32Field(name string, ref *int32) {
	v.fields = append(v.fields, bind(v, name, ref, false, false, "int32", convertToInt32, nil))
}

// AddRequiredStringField registers a field containing a required string connection value. The empty string will be treated as an "unset" value.
func (v *Validator) AddRequiredStringField(name string, ref *string) {
	v.fields = append(v.fields, bind(v, name, ref, true, false, "string", convertToString, nil))
}

// AddOptionalStringField registers a field containing an optional string connection value. The empty string will be treated as an "unset" value.
func (v *Validator) AddOptionalStringField(name string, ref *string) {
	v.fields = append(v.fields, bind(v, name, ref, false, false, "string", convertToString, nil))
}

// AddRequiredSecretField registers a field containing a required string connection secret. The empty string will be treated as an "unset" value.
func (v *Validator) AddRequiredSecretField(name string, ref *string) {
	// Note: secrets are always strings
	v.fields = append(v.fields, bind(v, name, ref, true, true, "string", convertToString, nil))
}

// AddRequiredSecretField registers a field containing an optional string connection secret. The empty string will be treated as an "unset" value.
func (v *Validator) AddOptionalSecretField(name string, ref *string) {
	// Note: secrets are always strings
	v.fields = append(v.fields, bind(v, name, ref, false, true, "string", convertToString, nil))
}

// AddOptionalAnyField registers a field containing any property value. The empty property will be treated as an "unset" value.
func (v *Validator) AddOptionalAnyField(name string, ref any) {
	v.recordValue(name, ref, false)
}

// AddComputedStringField registers a field containing a computed string connection value. The empty string will be treated as an "unset" value.
//
// The compute function will be called if the value is not already set or provided by the recipe. Inside the compute function
// it is safe to assume that other non-computed fields have been populated already.
//
// The compute function will not be called if a validation error has previously occurred.
func (v *Validator) AddComputedStringField(name string, ref *string, compute func() (string, *ValidationError)) {
	// Note: secrets are always strings
	v.computedFields = append(v.computedFields, bind(v, name, ref, false, false, "string", convertToString, compute))
}

// AddComputedBoolField registers a field containing a computed boolean connection value. The false value will be treated as an "unset" value.
//
// The compute function will be called if the value is not already set or provided by the recipe. Inside the compute function
// it is safe to assume that other non-computed fields have been populated already.
//
// The compute function will not be called if a validation error has previously occurred.
func (v *Validator) AddComputedBoolField(name string, ref *bool, compute func() (bool, *ValidationError)) {
	// Note: secrets are always strings
	v.computedFields = append(v.computedFields, bind(v, name, ref, false, false, "bool", convertToBool, compute))
}

// AddComputedSecretField registers a field containing a computed string connection secret. The empty string will be treated as an "unset" value.
//
// The compute function will be called if the secret is not already set or provided by the recipe. Inside the compute function
// it is safe to assume that other non-computed fields have been populated already.
//
// The compute function will not be called if a validation error has previously occurred.
func (v *Validator) AddComputedSecretField(name string, ref *string, compute func() (string, *ValidationError)) {
	// Note: secrets are always strings
	v.computedFields = append(v.computedFields, bind(v, name, ref, false, true, "string", convertToString, compute))
}

// SetAndValidate will bind fields from the recipe output, populate output resources, and populate connection values/secrets.
//
// This function returns *ValidationError for validation failures.
//
// After calling SetAndValidate, the connection values/secrets and output resources will be populated.
func (v *Validator) SetAndValidate(output *recipes.RecipeOutput) error {
	msgs := []string{}

	if output != nil {
		recipeResources, err := GetOutputResourcesFromRecipe(output)
		if err != nil {
			return err
		}

		*v.OutputResources = append(*v.OutputResources, recipeResources...)
	}

	if v.resourcesField != nil {
		userResources, err := GetOutputResourcesFromResourcesField(*v.resourcesField)
		if err != nil {
			return err
		}

		*v.OutputResources = append(*v.OutputResources, userResources...)
	}

	for _, field := range v.fields {
		msg := field(output)
		if msg != "" {
			msgs = append(msgs, msg)
		}
	}

	// Run computed fields iff there are no errors so far. Computed fields
	// get to read the state of all non-computed fields.
	if len(msgs) == 0 {
		for _, field := range v.computedFields {
			msg := field(output)
			if msg != "" {
				msgs = append(msgs, msg)
			}
		}
	}

	if len(msgs) == 1 {
		return &ValidationError{Message: msgs[0]}
	}

	if len(msgs) > 0 {
		msg := fmt.Sprintf("validation returned multiple errors:\n\n%v", strings.Join(msgs, "\n"))
		return &ValidationError{Message: msg}
	}

	return nil
}

func bind[T any](v *Validator, name string, ref *T, required bool, secret bool, typeName string, convert func(value any) (T, bool), compute func() (T, *ValidationError)) func(output *recipes.RecipeOutput) string {
	return func(output *recipes.RecipeOutput) string {
		valueKind := kindConnectionValue
		propertyPath := fmt.Sprintf(".properties.%v", name)
		if secret {
			valueKind = kindConnectionSecret
			propertyPath = fmt.Sprintf(".properties.secrets.%v", name)
		}

		existing := *ref
		if !reflect.ValueOf(existing).IsZero() {
			// Field is already set
			v.recordValue(name, *ref, secret)
			return ""
		}

		if output == nil {
			// Note: required and computed are mutually exclusive
			if required {
				return v.buildRequiredValueError(name, false, valueKind, propertyPath)
			}

			if compute != nil {
				return computeValue(v, name, ref, secret, compute)
			}

			return ""
		}

		// OK we have a recipe
		var value any
		var ok bool
		if secret {
			value, ok = output.Secrets[name]
		} else {
			value, ok = output.Values[name]
		}
		if !ok {
			// Note: required and computed are mutually exclusive
			if required {
				return v.buildRequiredValueError(name, true, valueKind, propertyPath)
			}

			if compute == nil {
				return "" // Optional
			}

			return computeValue(v, name, ref, secret, compute)
		}

		converted, ok := convert(value)
		if !ok {
			return v.buildTypeMismatchError(name, typeName, valueKind, value)
		}

		*ref = converted
		v.recordValue(name, converted, secret)
		return ""
	}
}

func computeValue[T any](v *Validator, name string, ref *T, secret bool, compute func() (T, *ValidationError)) string {
	value, err := compute()
	if err != nil {
		return err.Error()
	}

	*ref = value
	v.recordValue(name, value, secret)
	return ""
}

func convertToString(value any) (string, bool) {
	converted, ok := value.(string)
	return converted, ok
}

func convertToBool(value any) (bool, bool) {
	converted, ok := value.(bool)
	return converted, ok
}

func convertToInt32(value any) (int32, bool) {
	switch v := value.(type) {
	case int32:
		return v, true
	case int:
		return int32(v), true
	case float64:
		return int32(v), true
	default:
		return int32(0), false
	}
}

func (v *Validator) recordValue(name string, value any, secret bool) {
	if secret {
		v.ConnectionSecrets[name] = rpv1.SecretValueReference{Value: value.(string)}
	} else {
		v.ConnectionValues[name] = value
	}
}

func (v *Validator) buildTypeMismatchError(name string, typeName string, valueKind string, value any) string {
	return fmt.Sprintf("the %v %q provided by the recipe is expected to be a %s, got %T", valueKind, name, typeName, value)
}

func (v *Validator) buildRequiredValueError(name string, recipe bool, valueKind string, propertyPath string) string {
	if recipe {
		return fmt.Sprintf("the %v %q should be provided by the recipe, set '%v' to provide a value manually", valueKind, name, propertyPath)
	}

	return fmt.Sprintf("the %v %q must be provided when not using a recipe. Set '%v' to provide a value manually", valueKind, name, propertyPath)
}
