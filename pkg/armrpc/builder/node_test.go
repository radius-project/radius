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

package builder

import (
	"testing"

	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/stretchr/testify/require"
)

func TestAddResource(t *testing.T) {
	r := &ResourceNode{
		Name:     "Applications.Core",
		Kind:     NamespaceResourceKind,
		children: make(map[string]*ResourceNode),
	}

	child := r.AddResource("virtualMachines", &ResourceOption[*rpctest.TestResourceDataModel, rpctest.TestResourceDataModel]{})
	require.Equal(t, "virtualMachines", child.Name)
	require.Equal(t, TrackedResourceKind, child.Kind, "child resource of namespace should be a tracked resource")
	require.Len(t, r.children, 1, "should have one child resource")

	require.Panics(t, func() {
		_ = r.AddResource("virtualMachines", &ResourceOption[*rpctest.TestResourceDataModel, rpctest.TestResourceDataModel]{})
	}, "panic when adding a resource with the same name")

	nested := child.AddResource("disks", &ResourceOption[*rpctest.TestResourceDataModel, rpctest.TestResourceDataModel]{})
	_ = child.AddResource("cpus", &ResourceOption[*rpctest.TestResourceDataModel, rpctest.TestResourceDataModel]{})
	require.Equal(t, "disks", nested.Name)
	require.Equal(t, ProxyResourceKind, nested.Kind, "nested resource should be a proxy resource")
	require.Len(t, child.children, 2, "should have 2 child resource")
}
