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

package portforward

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/labels"
)

func Test_CreateLabelSelectorForApplication(t *testing.T) {
	// Create a label selector for the application "test-app"
	selector, err := CreateLabelSelectorForApplication("test-app")
	require.NoError(t, err)
	require.NotNil(t, selector)
	require.Equal(t, "radapp.io/application=test-app", selector.String())

	// Create a label selector for the application "another-test-app"
	selector, err = CreateLabelSelectorForApplication("another-test-app")
	require.NoError(t, err)
	require.NotNil(t, selector)
	require.Equal(t, "radapp.io/application=another-test-app", selector.String())
}

func Test_CreateLabelSelectorForDashboard(t *testing.T) {
	// Create a label selector for the dashboard
	selector, err := CreateLabelSelectorForDashboard()
	require.NoError(t, err)
	require.NotNil(t, selector)
	selector.Matches(labels.Set{
		"app.kubernetes.io/name":    "dashboard",
		"app.kubernetes.io/part-of": "radius",
	})
	require.Equal(t, "app.kubernetes.io/name=dashboard,app.kubernetes.io/part-of=radius", selector.String())

	require.NotEqual(t, "app.kubernetes.io/part-of=radius,app.kubernetes.io/name=dashboard", selector.String())
}
