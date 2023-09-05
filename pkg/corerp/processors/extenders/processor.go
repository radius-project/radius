package extenders

import (
	"context"
	"fmt"

	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/portableresources/processors"
	"github.com/radius-project/radius/pkg/recipes"
)

// Processor is a processor for Extender resources.
type Processor struct {
}

// Process implements the processors.Processor interface for Extender resources. It validates and merges output values from
// the recipe output with the existing values in the resource. It returns an error if the secret values are not of type string
// or if any of the other validations fail.
func (p *Processor) Process(ctx context.Context, resource *datamodel.Extender, options processors.Options) error {
	validator := processors.NewValidator(&resource.ComputedValues, &resource.SecretValues, &resource.Properties.Status.OutputResources)

	computedValues := mergeOutputValues(resource.Properties.AdditionalProperties, options.RecipeOutput, false)
	for k, val := range computedValues {
		value := val
		validator.AddOptionalAnyField(k, value)
	}

	secretValues := mergeOutputValues(resource.Properties.Secrets, options.RecipeOutput, true)
	for k, val := range secretValues {
		if secret, ok := val.(string); !ok {
			return &processors.ValidationError{Message: fmt.Sprintf("secret '%s' must be of type string", k)}
		} else {
			value := secret
			validator.AddOptionalSecretField(k, &value)
		}
	}

	err := validator.SetAndValidate(options.RecipeOutput)
	if err != nil {
		return err
	}

	if options.RecipeOutput != nil {
		resource.Properties.AdditionalProperties = options.RecipeOutput.Values
		resource.Properties.Secrets = options.RecipeOutput.Secrets
	}

	return nil
}

func mergeOutputValues(properties map[string]any, recipeOutput *recipes.RecipeOutput, secret bool) map[string]any {
	values := make(map[string]any)
	for k, val := range properties {
		values[k] = val
	}
	if recipeOutput == nil {
		return values
	}

	var recipeProperties map[string]any
	recipeProperties = recipeOutput.Values
	if secret {
		recipeProperties = recipeOutput.Secrets
	}

	for k, val := range recipeProperties {
		values[k] = val
	}
	return values
}
