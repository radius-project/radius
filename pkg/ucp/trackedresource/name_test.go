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

package trackedresource

import (
	"testing"

	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/require"
)

var (
	testID = resources.MustParse("/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/applications/test-app")
)

func Test_NameFor(t *testing.T) {
	name := NameFor(testID)
	require.Equal(t, "test-app-303153687ee5adbcf353bc6c2caa4373f31e04c6", name)
}

func Test_IDFor(t *testing.T) {
	id := IDFor(testID)
	require.Equal(t, resources.MustParse("/planes/radius/local/resourceGroups/test-group/providers/System.Resources/resources/test-app-303153687ee5adbcf353bc6c2caa4373f31e04c6"), id)
}
