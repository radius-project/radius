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

package backend

import (
	"context"

	"github.com/radius-project/radius/pkg/dynamicrp/datamodel"
	"github.com/radius-project/radius/pkg/portableresources/processors"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
)

var _ processors.ResourceProcessor[*datamodel.DynamicResource, datamodel.DynamicResource] = (*dynamicProcessor)(nil)

type dynamicProcessor struct {
}

func (d *dynamicProcessor) Delete(ctx context.Context, resource *datamodel.DynamicResource, options processors.Options) error {
	return nil
}

func (d *dynamicProcessor) Process(ctx context.Context, resource *datamodel.DynamicResource, options processors.Options) error {
	computedValues := map[string]any{}
	secretValues := map[string]rpv1.SecretValueReference{}
	outputResources := []rpv1.OutputResource{}
	status := rpv1.RecipeStatus{}

	validator := processors.NewValidator(&computedValues, &secretValues, &outputResources, &status)

	// TODO: loop over schema and add to validator - right now this bypasses validation.
	for key, value := range options.RecipeOutput.Values {
		value := value
		validator.AddOptionalAnyField(key, &value)
	}
	for key, value := range options.RecipeOutput.Secrets {
		value := value.(string)
		validator.AddOptionalSecretField(key, &value)
	}

	err := validator.SetAndValidate(options.RecipeOutput)
	if err != nil {
		return err
	}

	err = resource.ApplyDeploymentOutput(rpv1.DeploymentOutput{DeployedOutputResources: outputResources, ComputedValues: computedValues, SecretValues: secretValues})
	if err != nil {
		return err
	}

	return nil
}
