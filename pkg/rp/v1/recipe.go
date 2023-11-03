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

package v1

// RecipeStatus defines the status of the recipe
type RecipeStatus struct {
	// TemplateKind specifies the kind of template used for the recipe.
	TemplateKind string `json:"templateKind,omitempty"`

	// TemplatePath specifies the path of the template used for the recipe.
	TemplatePath string `json:"templatePath,omitempty"`

	// TemplateVersion specifies the version of the template used for the recipe.
	TemplateVersion string `json:"templateVersion,omitempty"`
}
