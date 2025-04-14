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

package datamodel

import (
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
)

type UDT struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties UDTProperties `json:"properties"`
}

type UDTProperties struct {
	Application string         `json:"application"`
	Environment string         `json:"environment"`
	Recipe      Recipe         `json:"recipe"`
	Status      ResourceStatus `json:"status"`
}
type Recipe struct {
	Name         string `json:"name"`
	RecipeStatus string `json:"recipeStatus"`
	TemplateKind string `json:"templateKind"`
	TemplatePath string `json:"templatePath"`
}

type ResourceStatus struct {
	Binding         map[string]any        `json:"binding"`
	OutputResources []rpv1.OutputResource `json:"outputResources"`
	Recipe          Recipe                `json:"recipe"`
}
