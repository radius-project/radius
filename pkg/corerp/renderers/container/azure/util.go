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

package azure

import (
	"github.com/project-radius/radius/pkg/kubernetes"
)

const (
	// Separator represents the resource name separator.
	Separator = "-"
)

// # Function Explanation
//
// MakeResourceName creates a normalized resource name by combining the prefix, name and separator.
// For instance, when user uses keyvault persistent volume, RP will
// auto-provision per-container managed identity in the resource group
// which is specified by environment resource. In this case,
// RP uses application name as prefix to avoid the name conflict in the same
// resource group.
func MakeResourceName(prefix, name, separator string) string {
	if name == "" {
		panic("name is empty.")
	}
	if prefix != "" {
		prefix += separator
	}
	return kubernetes.NormalizeResourceName(prefix + name)
}
