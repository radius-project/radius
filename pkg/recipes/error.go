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

package recipes

import (
	"fmt"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/recipes/util"
)

type RecipeError struct {
	ErrorDetails     v1.ErrorDetails
	DeploymentStatus util.RecipeDeploymentStatus
}

// Error returns an error string describing the error code and message.
func (r *RecipeError) Error() string {
	return fmt.Sprintf("code %v: err %v", r.ErrorDetails.Code, r.ErrorDetails.Message)
}

func (e *RecipeError) Is(target error) bool {
	_, ok := target.(*RecipeError)
	return ok
}

// NewRecipeError creates a new RecipeError error with a given code, message and error details.
func NewRecipeError(code string, message string, deploymentStatus util.RecipeDeploymentStatus, details ...*v1.ErrorDetails) *RecipeError {
	err := new(RecipeError)
	err.ErrorDetails.Message = message
	err.ErrorDetails.Code = code
	err.DeploymentStatus = deploymentStatus
	for _, val := range details {
		if val != nil {
			err.ErrorDetails.Details = append(err.ErrorDetails.Details, *val)
		}
	}

	return err
}

// GetRecipeErrorDetails is used to get ErrorDetails if error is of type RecipeError else returns nil.
func GetRecipeErrorDetails(err error) *v1.ErrorDetails {
	recipeError, _ := err.(*RecipeError)
	if recipeError != nil {
		return &recipeError.ErrorDetails
	}

	return nil
}
