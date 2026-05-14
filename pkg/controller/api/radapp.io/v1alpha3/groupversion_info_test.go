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

package v1alpha3

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// TestAddToScheme verifies that AddToScheme registers all of this API group's
// types under the correct GroupVersion, including the metav1 list-machinery
// types that List operations depend on. This guards against regressions in the
// scheme registration plumbing (e.g. forgetting metav1.AddToGroupVersion or
// failing to register a new type via AddKnownTypes).
func TestAddToScheme(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, AddToScheme(scheme))

	// Each Radius CRD type (and its corresponding List type) must be registered
	// under GroupVersion with the matching Kind.
	cases := []struct {
		obj  runtime.Object
		kind string
	}{
		{&Recipe{}, "Recipe"},
		{&RecipeList{}, "RecipeList"},
		{&DeploymentTemplate{}, "DeploymentTemplate"},
		{&DeploymentTemplateList{}, "DeploymentTemplateList"},
		{&DeploymentResource{}, "DeploymentResource"},
		{&DeploymentResourceList{}, "DeploymentResourceList"},
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			gvks, unversioned, err := scheme.ObjectKinds(tc.obj)
			require.NoError(t, err)
			require.False(t, unversioned)
			require.Len(t, gvks, 1)
			require.Equal(t, GroupVersion, gvks[0].GroupVersion())
			require.Equal(t, tc.kind, gvks[0].Kind)
		})
	}

	// metav1.AddToGroupVersion must be invoked so list/watch machinery
	// (ListOptions, etc.) works for this GroupVersion.
	gvks, _, err := scheme.ObjectKinds(&metav1.ListOptions{})
	require.NoError(t, err)
	require.Contains(t, gvks, GroupVersion.WithKind("ListOptions"))
}
