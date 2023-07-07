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

package config

const (
	moduleSourceKey  = "source"
	moduleVersionKey = "version"

	mainConfigFileName = "main.tf.json"
)

// TerraformConfig represents the Terraform configuration file structure for properties populated in the configuration by Radius.
type TerraformConfig struct {
	// Module is the Terraform module configuration.
	Module map[string]any `json:"module"`
}
