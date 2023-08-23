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

package resourcemodel

import (
	"fmt"
)

// Providers supported by Radius
// The RP will be able to support a resource only if the corresponding provider is configured with the RP
const (
	ProviderAzure      = "azure"
	ProviderAWS        = "aws"
	ProviderRadius     = "radius"
	ProviderKubernetes = "kubernetes"
)

// ResourceType determines the type of the resource and the provider domain for the resource
type ResourceType struct {
	Type     string `json:"type"`
	Provider string `json:"provider"`
}

// String returns a string representation of the ResourceType instance.
func (r ResourceType) String() string {
	return fmt.Sprintf("Provider: %s, Type: %s", r.Provider, r.Type)
}
