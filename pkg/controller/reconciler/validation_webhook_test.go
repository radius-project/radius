/*
Copyright 2023 The KEDA Authors

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
	"testing"

	"github.com/radius-project/radius/test/testcontext"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

func TestValidateRecipeWithValidType(t *testing.T) {
	ctx := testcontext.New(t)
	_, client := SetupRecipeTest(t)

	name := types.NamespacedName{Namespace: "recipe-validtype", Name: "test-recipe-validtype"}
	err := client.Create(ctx, &corev1.Namespace{ObjectMeta: ctrl.ObjectMeta{Name: name.Namespace}})
	require.NoError(t, err)

	recipe := makeRecipe(name, "Applications.Core/extenders")
	err = client.Create(ctx, recipe)
	require.NoError(t, err)

	err = client.Delete(ctx, recipe)
	require.NoError(t, err)
}

func TestValidateRecipeWithInvalidValidType(t *testing.T) {
	ctx := testcontext.New(t)
	_, client := SetupRecipeTest(t)

	name := types.NamespacedName{Namespace: "recipe-invalidtype", Name: "test-recipe-invalidtype"}
	err := client.Create(ctx, &corev1.Namespace{ObjectMeta: ctrl.ObjectMeta{Name: name.Namespace}})
	require.NoError(t, err)

	recipe := makeRecipe(name, "Applications.Core/invalidtype")
	err = client.Create(ctx, recipe)
	// TODO: need check for type/content of error
	require.Error(t, err)
}
