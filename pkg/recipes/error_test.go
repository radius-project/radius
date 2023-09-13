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
	"errors"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/recipes/util"
	"github.com/stretchr/testify/require"
)

func TestNewRecipeError(t *testing.T) {
	errorTests := []struct {
		name         string
		errorCode    string
		errorMessage string
		errorDetails *v1.ErrorDetails
		expectedErr  RecipeError
	}{
		{
			name:         "error with details",
			errorCode:    RecipeDeploymentFailed,
			errorMessage: "test-recipe-deployment-failed-message",
			errorDetails: &v1.ErrorDetails{
				Code:    RecipeLanguageFailure,
				Message: "test-recipe-language-failure-message",
			},
			expectedErr: RecipeError{
				v1.ErrorDetails{
					Code:    RecipeDeploymentFailed,
					Message: "test-recipe-deployment-failed-message",
					Details: []v1.ErrorDetails{
						{
							Code:    RecipeLanguageFailure,
							Message: "test-recipe-language-failure-message",
						},
					},
				},
				util.RecipeSetupError,
			},
		},
		{
			name:         "error without details",
			errorCode:    RecipeDeploymentFailed,
			errorMessage: "test-recipe-deployment-failed-message",
			errorDetails: nil,
			expectedErr: RecipeError{
				v1.ErrorDetails{
					Code:    RecipeDeploymentFailed,
					Message: "test-recipe-deployment-failed-message",
				},
				util.ExecutionError,
			},
		},
	}
	for _, tc := range errorTests {
		err := NewRecipeError(tc.errorCode, tc.errorMessage, tc.expectedErr.DeploymentStatus, tc.errorDetails)
		require.Equal(t, err, &tc.expectedErr)
	}
}

func TestGetRecipeErrorDetails(t *testing.T) {
	errorTests := []struct {
		name            string
		err             error
		expErrorDetails *v1.ErrorDetails
	}{
		{
			name:            "",
			err:             errors.New("test-error"),
			expErrorDetails: nil,
		},
		{
			name: "",
			err: &RecipeError{
				v1.ErrorDetails{
					Code:    RecipeDeploymentFailed,
					Message: "test-recipe-deployment-failed-message",
				},
				util.RecipeSetupError,
			},
			expErrorDetails: &v1.ErrorDetails{
				Code:    RecipeDeploymentFailed,
				Message: "test-recipe-deployment-failed-message",
			},
		},
	}
	for _, tc := range errorTests {
		details := GetRecipeErrorDetails(tc.err)
		require.Equal(t, details, tc.expErrorDetails)
	}
}
