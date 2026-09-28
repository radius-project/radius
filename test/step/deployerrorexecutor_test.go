/*
Copyright 2026 The Radius Authors.

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

package step

import (
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/test/radcli"
	"github.com/stretchr/testify/require"
)

func Test_DeploymentErrorDetail_MatchesExactly(t *testing.T) {
	t.Parallel()

	expected := DeploymentErrorDetail{
		Code: "DeploymentFailed",
		Details: []DeploymentErrorDetail{{
			Code: "ResourceDeploymentFailure",
			Details: []DeploymentErrorDetail{{
				Code:            "RecipeDeploymentFailed",
				MessageContains: "failed to deploy recipe default of type Test.Resources/userTypeAlpha",
				Details: []DeploymentErrorDetail{{
					Code:            "DeploymentFailed",
					MessageContains: "At least one resource deployment operation failed",
					Details: []DeploymentErrorDetail{{
						Code:            "BadRequest",
						MessageContains: "Namespace parameter required.",
					}},
				}},
			}},
		}},
	}

	for _, tt := range []struct {
		name        string
		mutate      func([]*v1.ErrorDetails)
		exactMatch  bool
		subsetMatch bool
	}{
		{name: "structured leaf with legitimate wrappers", exactMatch: true, subsetMatch: true},
		{
			name: "duplicate complete branch",
			mutate: func(nodes []*v1.ErrorDetails) {
				nodes[0].Details = append(nodes[0].Details, nodes[1])
			},
			subsetMatch: true,
		},
		{
			name: "matching branch alongside incomplete duplicate prefix",
			mutate: func(nodes []*v1.ErrorDetails) {
				nodes[0].Details = append(nodes[0].Details, &v1.ErrorDetails{
					Code: "ResourceDeploymentFailure",
					Details: []*v1.ErrorDetails{{
						Code:    "RecipeDeploymentFailed",
						Message: nodes[2].Message,
					}},
				})
			},
			subsetMatch: true,
		},
		{
			name: "duplicate recipe wrapper",
			mutate: func(nodes []*v1.ErrorDetails) {
				nodes[1].Details = append(nodes[1].Details, nodes[2])
			},
			subsetMatch: true,
		},
		{
			name: "duplicate nested deployment wrapper",
			mutate: func(nodes []*v1.ErrorDetails) {
				nodes[2].Details = append(nodes[2].Details, nodes[3])
			},
			subsetMatch: true,
		},
		{
			name: "duplicate provider leaf",
			mutate: func(nodes []*v1.ErrorDetails) {
				nodes[3].Details = append(nodes[3].Details, nodes[4])
			},
			subsetMatch: true,
		},
		{
			name: "matching leaf alongside divergent provider error",
			mutate: func(nodes []*v1.ErrorDetails) {
				nodes[3].Details = append(nodes[3].Details, &v1.ErrorDetails{
					Code:    "Conflict",
					Message: "A different provider operation failed.",
				})
			},
			subsetMatch: true,
		},
		{
			name: "structured leaf alongside opaque duplicate",
			mutate: func(nodes []*v1.ErrorDetails) {
				nodes[3].Details = append(nodes[3].Details, &v1.ErrorDetails{
					Message: `{"error":{"code":"BadRequest","message":"Namespace parameter required."}}`,
				})
			},
			subsetMatch: true,
		},
		{
			name: "unexpected detail below provider leaf",
			mutate: func(nodes []*v1.ErrorDetails) {
				nodes[4].Details = []*v1.ErrorDetails{{Code: "BadRequest", Message: nodes[4].Message}}
			},
			subsetMatch: true,
		},
		{
			name: "missing recipe wrapper",
			mutate: func(nodes []*v1.ErrorDetails) {
				nodes[1].Details = nodes[2].Details
			},
		},
		{
			name: "missing nested deployment wrapper",
			mutate: func(nodes []*v1.ErrorDetails) {
				nodes[2].Details = nodes[3].Details
			},
		},
		{
			name: "incomplete branch without provider leaf",
			mutate: func(nodes []*v1.ErrorDetails) {
				nodes[3].Details = nil
			},
		},
		{
			name: "historical opaque leaf",
			mutate: func(nodes []*v1.ErrorDetails) {
				nodes[4].Code = ""
				nodes[4].Message = `{"error":{"code":"BadRequest","message":"Namespace parameter required."}}`
			},
		},
		{
			name: "wrong top-level code",
			mutate: func(nodes []*v1.ErrorDetails) {
				nodes[0].Code = "BadRequest"
			},
		},
		{
			name: "wrong provider message",
			mutate: func(nodes []*v1.ErrorDetails) {
				nodes[4].Message = "A different parameter is required."
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			nodes := []*v1.ErrorDetails{
				{Code: "DeploymentFailed"},
				{Code: "ResourceDeploymentFailure"},
				{Code: "RecipeDeploymentFailed", Message: "failed to deploy recipe default of type Test.Resources/userTypeAlpha"},
				{Code: "DeploymentFailed", Message: "At least one resource deployment operation failed. Inspect the deployment operations."},
				{Code: "BadRequest", Message: "Invalid configuration: Namespace parameter required. Set the environment's Kubernetes namespace."},
			}
			for i := 0; i < len(nodes)-1; i++ {
				nodes[i].Details = []*v1.ErrorDetails{nodes[i+1]}
			}
			if tt.mutate != nil {
				tt.mutate(nodes)
			}

			require.Equal(t, tt.exactMatch, expected.MatchesExactly(nodes[0]))
			require.Equal(t, tt.subsetMatch, expected.Matches(nodes[0]), "default subset matching must remain unchanged")
			if tt.exactMatch {
				ValidateExactError(expected)(t, &radcli.CLIError{ErrorResponse: v1.ErrorResponse{Error: nodes[0]}})
			}
		})
	}

	t.Run("nil error", func(t *testing.T) {
		require.False(t, expected.MatchesExactly(nil))
	})
	t.Run("nil detail", func(t *testing.T) {
		require.False(t, expected.MatchesExactly(&v1.ErrorDetails{
			Code: "DeploymentFailed", Details: []*v1.ErrorDetails{nil},
		}))
	})
}
