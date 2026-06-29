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

package setup

import (
	"context"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/cli/clients"
	corerp "github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/to"
	ucp "github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
)

// EnsureResourceGroup creates or updates a Radius resource group on the given UCP plane using
// CreateOrUpdate semantics. Re-running on an existing group is safe.
func EnsureResourceGroup(ctx context.Context, client clients.ApplicationsManagementClient, planeName, groupName string) error {
	return client.CreateOrUpdateResourceGroup(ctx, planeName, groupName, &ucp.ResourceGroupResource{
		Location: to.Ptr(v1.LocationGlobal),
	})
}

// EnsureEnvironment creates or updates a Radius environment configured to deploy into the given
// Kubernetes namespace. The optional providers and recipes are attached to the environment when
// non-nil. Re-running on an existing environment is safe.
func EnsureEnvironment(
	ctx context.Context,
	client clients.ApplicationsManagementClient,
	envName string,
	namespace string,
	providers *corerp.Providers,
	recipes map[string]map[string]corerp.RecipePropertiesClassification,
) error {
	properties := corerp.EnvironmentProperties{
		Compute: &corerp.KubernetesCompute{
			Namespace: to.Ptr(namespace),
		},
		Providers: providers,
		Recipes:   recipes,
	}

	return client.CreateOrUpdateEnvironment(ctx, envName, &corerp.EnvironmentResource{
		Location:   to.Ptr(v1.LocationGlobal),
		Properties: &properties,
	})
}
