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

package backends

import (
	"fmt"

	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
)

// CloudBackend renders a built-in cloud backend. Terraform owns state access and locking.
type CloudBackend struct {
	Settings *datamodel.TerraformBackend
}

// BuildBackend renders only location and locking options, never authentication material.
func (b CloudBackend) BuildBackend(resource *recipes.ResourceMetadata) (map[string]any, error) {
	if b.Settings == nil || resource == nil {
		return nil, fmt.Errorf("cloud backend settings and resource metadata are required")
	}
	if err := b.Settings.Validate(); err != nil {
		return nil, err
	}
	// Reuse the current resource identity, not Kubernetes's legacy-secret discovery.
	hash, err := generateSecretSuffix(resource)
	if err != nil {
		return nil, err
	}
	key := b.Settings.EffectiveKeyPrefix() + "/" + hash + ".tfstate"
	if b.Settings.Type == "s3" {
		return map[string]any{"s3": map[string]any{
			"bucket":       b.Settings.Bucket,
			"region":       b.Settings.Region,
			"key":          key,
			"use_lockfile": true,
		}}, nil
	}
	return map[string]any{"azurerm": map[string]any{
		"storage_account_name": b.Settings.StorageAccountName,
		"container_name":       b.Settings.ContainerName,
		"key":                  key,
		"use_azuread_auth":     true,
	}}, nil
}
