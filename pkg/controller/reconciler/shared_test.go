/*
Copyright 2023.

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

package reconciler

import (
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	"github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/to"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

func createEnvironment(radius *mockRadiusClient) {
	radius.Update(func() {
		radius.environments["/planes/radius/local/resourceGroups/default/providers/Applications.Core/environments/default"] = v20231001preview.EnvironmentResource{
			ID:       to.Ptr("/planes/radius/local/resourceGroups/default/providers/Applications.Core/environments/default"),
			Name:     to.Ptr("default"),
			Location: to.Ptr(v1.LocationGlobal),
		}
	})
}

func makeRecipe(name types.NamespacedName, resourceType string) *radappiov1alpha3.Recipe {
	return &radappiov1alpha3.Recipe{
		ObjectMeta: ctrl.ObjectMeta{
			Namespace: name.Namespace,
			Name:      name.Name,
		},
		Spec: radappiov1alpha3.RecipeSpec{
			Type: resourceType,
		},
	}
}
