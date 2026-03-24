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

package radinit

import (
	"testing"

	corerp "github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	ds_ctrl "github.com/radius-project/radius/pkg/datastoresrp/frontend/controller"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/to"
	"github.com/stretchr/testify/require"
)

func Test_getRecipeProperties(t *testing.T) {
	type args struct {
		devRecipe DevRecipe
		tag       string
	}
	tests := []struct {
		name string
		args args
		want map[string]corerp.RecipePropertiesClassification
	}{
		{
			"Mongo Database Dev Recipe",
			args{
				DevRecipe{
					"mongodatabases",
					ds_ctrl.MongoDatabasesResourceType,
					RecipeRepositoryPrefix + "mongodatabases",
				},
				"0.20",
			},
			map[string]corerp.RecipePropertiesClassification{
				"default": &corerp.BicepRecipeProperties{
					TemplateKind: to.Ptr(recipes.TemplateKindBicep),
					TemplatePath: to.Ptr(RecipeRepositoryPrefix + "mongodatabases:0.20"),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getRecipeProperties(tt.args.devRecipe, tt.args.tag)
			require.Equal(t, tt.want, got, "getRecipeProperties() = %v, want %v", got, tt.want)
		})
	}
}
