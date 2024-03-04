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

package graph

import (
	"github.com/radius-project/radius/pkg/resourcemodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

func providerFromID(id string) string {
	parsed, err := resources.ParseResource(id)
	if err != nil {
		return ""
	}

	if len(parsed.ScopeSegments()) > 0 && parsed.IsUCPQualified() {
		return parsed.ScopeSegments()[0].Type
	} else if len(parsed.ScopeSegments()) > 0 {
		// Relative Resource ID (ARM)
		return resourcemodel.ProviderAzure
	}

	return ""
}
