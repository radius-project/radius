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
	"errors"
	"testing"
	"time"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	asyncctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/frontend/defaultoperation"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/stretchr/testify/require"
)

var (
	testBuildOptions = BuildOptions{
		ResourceType:        "Applications.Compute/virtualMachines",
		ResourceNamePattern: "applications.compute/virtualmachines",
	}

	testBuildOptionsWithName = BuildOptions{
		ResourceType:        "Applications.Compute/virtualMachines",
		ResourceNamePattern: "applications.compute/virtualmachines",
		ParameterName:       "{virtualMachineName}",
	}
)

func TestGetOrDefaultAsyncOperationTimeout(t *testing.T) {
	var zeroDuration time.Duration
	require.Equal(t, time.Duration(120)*time.Second, getOrDefaultAsyncOperationTimeout(zeroDuration))
	require.Equal(t, time.Duration(1)*time.Minute, getOrDefaultAsyncOperationTimeout(time.Duration(1)*time.Minute))
}

func TestGetOrDefaultRetryAfter(t *testing.T) {
	var zeroDuration time.Duration
	require.Equal(t, time.Duration(60)*time.Second, getOrDefaultRetryAfter(zeroDuration))
	require.Equal(t, time.Duration(1)*time.Minute, getOrDefaultRetryAfter(time.Duration(1)*time.Minute))
}

func TestResourceOption_LinkResource(t *testing.T) {
	node := &ResourceNode{Name: "virtualMachines", Kind: TrackedResourceKind}
	option := &ResourceOption[*rpctest.TestResourceDataModel, rpctest.TestResourceDataModel]{}
	option.LinkResource(node)
	require.Equal(t, node, option.linkedNode)
}

func TestResourceOption_ParamName(t *testing.T) {
	t.Run("custom parameter name", func(t *testing.T) {
		option := &ResourceOption[*rpctest.TestResourceDataModel, rpctest.TestResourceDataModel]{
			ResourceParamName: "virtualMachineName",
		}
		require.Equal(t, "virtualMachineName", option.ParamName())
	})

	t.Run("plural resource type name", func(t *testing.T) {
		node := &ResourceNode{Name: "virtualMachines", Kind: TrackedResourceKind}
		option := &ResourceOption[*rpctest.TestResourceDataModel, rpctest.TestResourceDataModel]{}
		option.LinkResource(node)
		require.Equal(t, "virtualMachineName", option.ParamName())
	})

	t.Run("plural resource type name without s", func(t *testing.T) {
		node := &ResourceNode{Name: "dice", Kind: TrackedResourceKind}
		option := &ResourceOption[*rpctest.TestResourceDataModel, rpctest.TestResourceDataModel]{}
		option.LinkResource(node)
		require.Equal(t, "diceName", option.ParamName())
	})
}

func TestResourceOption_ListPlaneOutput(t *testing.T) {
	t.Run("disabled is true", func(t *testing.T) {
		node := &ResourceNode{Name: "virtualMachines", Kind: TrackedResourceKind}
		option := &ResourceOption[*rpctest.TestResourceDataModel, rpctest.TestResourceDataModel]{
			linkedNode: node,
			ListPlane: Operation[rpctest.TestResourceDataModel]{
				Disabled: true,
			},
		}
		require.Nil(t, option.listPlaneOutput(BuildOptions{}))
	})

	t.Run("non tracked resource disabled operation", func(t *testing.T) {
		node := &ResourceNode{Name: "virtualMachines", Kind: ProxyResourceKind}
		option := &ResourceOption[*rpctest.TestResourceDataModel, rpctest.TestResourceDataModel]{
			linkedNode: node,
			ListPlane:  Operation[rpctest.TestResourceDataModel]{},
		}
		require.Nil(t, option.listPlaneOutput(BuildOptions{}))
	})

	t.Run("custom controller", func(t *testing.T) {
		node := &ResourceNode{Name: "virtualMachines", Kind: TrackedResourceKind}
		option := &ResourceOption[*rpctest.TestResourceDataModel, rpctest.TestResourceDataModel]{
			linkedNode: node,
			ListPlane: Operation[rpctest.TestResourceDataModel]{
				APIController: func(opt controller.Options) (controller.Controller, error) {
					return nil, errors.New("ok")
				},
			},
		}
		h := option.listPlaneOutput(testBuildOptions)
		require.NotNil(t, h)
		_, err := h.APIController(controller.Options{})
		require.EqualError(t, err, "ok")
		require.Equal(t, v1.OperationPlaneScopeList, h.Method)
		require.Equal(t, "Applications.Compute/virtualMachines", h.ResourceType)
		require.Equal(t, "applications.compute/virtualmachines", h.ResourceNamePattern)
		require.Empty(t, h.Path)
	})

	t.Run("default controller", func(t *testing.T) {
		node := &ResourceNode{Name: "virtualMachines", Kind: TrackedResourceKind}
		option := &ResourceOption[*rpctest.TestResourceDataModel, rpctest.TestResourceDataModel]{
			linkedNode: node,
			ListPlane:  Operation[rpctest.TestResourceDataModel]{},
		}
		h := option.listPlaneOutput(testBuildOptions)
		require.NotNil(t, h)
		require.NotNil(t, h.APIController)
		require.Equal(t, v1.OperationPlaneScopeList, h.Method)
		require.Equal(t, "Applications.Compute/virtualMachines", h.ResourceType)
		require.Equal(t, "applications.compute/virtualmachines", h.ResourceNamePattern)
		require.Empty(t, h.Path)
	})
}

func TestResourceOption_ListOutput(t *testing.T) {
	node := &ResourceNode{Name: "virtualMachines", Kind: TrackedResourceKind}
	t.Run("disabled is true", func(t *testing.T) {
		option := &ResourceOption[*rpctest.TestResourceDataModel, rpctest.TestResourceDataModel]{
			linkedNode: node,
			List: Operation[rpctest.TestResourceDataModel]{
				Disabled: true,
			},
		}
		require.Nil(t, option.listOutput(BuildOptions{}))
	})

	t.Run("custom controller", func(t *testing.T) {
		option := &ResourceOption[*rpctest.TestResourceDataModel, rpctest.TestResourceDataModel]{
			linkedNode: node,
			List: Operation[rpctest.TestResourceDataModel]{
				APIController: func(opt controller.Options) (controller.Controller, error) {
					return nil, errors.New("ok")
				},
			},
		}
		h := option.listOutput(testBuildOptions)
		require.NotNil(t, h)
		_, err := h.APIController(controller.Options{})
		require.EqualError(t, err, "ok")
		require.Equal(t, v1.OperationList, h.Method)
		require.Equal(t, "Applications.Compute/virtualMachines", h.ResourceType)
		require.Equal(t, "applications.compute/virtualmachines", h.ResourceNamePattern)
		require.Empty(t, h.Path)
	})

	t.Run("default controller", func(t *testing.T) {
		option := &ResourceOption[*rpctest.TestResourceDataModel, rpctest.TestResourceDataModel]{
			linkedNode: node,
			List:       Operation[rpctest.TestResourceDataModel]{},
		}
		h := option.listOutput(testBuildOptions)
		require.NotNil(t, h)
		require.NotNil(t, h.APIController)
		require.Equal(t, v1.OperationList, h.Method)
		require.Equal(t, "Applications.Compute/virtualMachines", h.ResourceType)
		require.Equal(t, "applications.compute/virtualmachines", h.ResourceNamePattern)
		require.Empty(t, h.Path)
	})
}

func TestResourceOption_GetOutput(t *testing.T) {
	node := &ResourceNode{Name: "virtualMachines", Kind: TrackedResourceKind}

	t.Run("disabled is true", func(t *testing.T) {
		option := &ResourceOption[*rpctest.TestResourceDataModel, rpctest.TestResourceDataModel]{
			linkedNode: node,
			Get: Operation[rpctest.TestResourceDataModel]{
				Disabled: true,
			},
		}
		require.Nil(t, option.getOutput(BuildOptions{}))
	})

	t.Run("custom controller", func(t *testing.T) {
		option := &ResourceOption[*rpctest.TestResourceDataModel, rpctest.TestResourceDataModel]{
			linkedNode: node,
			Get: Operation[rpctest.TestResourceDataModel]{
				APIController: func(opt controller.Options) (controller.Controller, error) {
					return nil, errors.New("ok")
				},
			},
		}
		h := option.getOutput(testBuildOptionsWithName)
		require.NotNil(t, h)
		_, err := h.APIController(controller.Options{})
		require.EqualError(t, err, "ok")
		require.Equal(t, v1.OperationGet, h.Method)
		require.Equal(t, "Applications.Compute/virtualMachines", h.ResourceType)
		require.Equal(t, "applications.compute/virtualmachines/{virtualMachineName}", h.ResourceNamePattern)
		require.Empty(t, h.Path)
	})

	t.Run("default controller", func(t *testing.T) {
		option := &ResourceOption[*rpctest.TestResourceDataModel, rpctest.TestResourceDataModel]{
			linkedNode: node,
			Get:        Operation[rpctest.TestResourceDataModel]{},
		}
		h := option.getOutput(testBuildOptionsWithName)
		require.NotNil(t, h)
		require.NotNil(t, h.APIController)
		require.Equal(t, v1.OperationGet, h.Method)
		require.Equal(t, "Applications.Compute/virtualMachines", h.ResourceType)
		require.Equal(t, "applications.compute/virtualmachines/{virtualMachineName}", h.ResourceNamePattern)
		require.Empty(t, h.Path)
	})
}

func TestResourceOption_PutOutput(t *testing.T) {
	node := &ResourceNode{Name: "virtualMachines", Kind: TrackedResourceKind}

	t.Run("disabled is true", func(t *testing.T) {
		option := &ResourceOption[*rpctest.TestResourceDataModel, rpctest.TestResourceDataModel]{
			linkedNode: node,
			Put: Operation[rpctest.TestResourceDataModel]{
				Disabled: true,
			},
		}
		require.Nil(t, option.putOutput(BuildOptions{}))
	})

	t.Run("custom controller", func(t *testing.T) {
		option := &ResourceOption[*rpctest.TestResourceDataModel, rpctest.TestResourceDataModel]{
			linkedNode: node,
			Put: Operation[rpctest.TestResourceDataModel]{
				APIController: func(opt controller.Options) (controller.Controller, error) {
					return nil, errors.New("ok")
				},
			},
		}
		h := option.putOutput(testBuildOptionsWithName)
		require.NotNil(t, h)
		_, err := h.APIController(controller.Options{})
		require.EqualError(t, err, "ok")
		require.Equal(t, v1.OperationPut, h.Method)
		require.Equal(t, "Applications.Compute/virtualMachines", h.ResourceType)
		require.Equal(t, "applications.compute/virtualmachines/{virtualMachineName}", h.ResourceNamePattern)
		require.Empty(t, h.Path)
	})

	t.Run("default sync controller", func(t *testing.T) {
		option := &ResourceOption[*rpctest.TestResourceDataModel, rpctest.TestResourceDataModel]{
			linkedNode: node,
			Put:        Operation[rpctest.TestResourceDataModel]{},
		}
		h := option.putOutput(testBuildOptionsWithName)
		require.NotNil(t, h)
		require.Equal(t, v1.OperationPut, h.Method)

		api, err := h.APIController(controller.Options{})
		require.NoError(t, err)
		_, ok := api.(*defaultoperation.DefaultSyncPut[*rpctest.TestResourceDataModel, rpctest.TestResourceDataModel])
		require.True(t, ok)
		require.Equal(t, "Applications.Compute/virtualMachines", h.ResourceType)
		require.Equal(t, "applications.compute/virtualmachines/{virtualMachineName}", h.ResourceNamePattern)
		require.Empty(t, h.Path)
	})

	t.Run("default async controller", func(t *testing.T) {
		option := &ResourceOption[*rpctest.TestResourceDataModel, rpctest.TestResourceDataModel]{
			linkedNode: node,
			Put: Operation[rpctest.TestResourceDataModel]{
				AsyncJobController: func(opts asyncctrl.Options) (asyncctrl.Controller, error) {
					return nil, nil
				},
			},
		}
		h := option.putOutput(testBuildOptionsWithName)
		require.NotNil(t, h)
		require.Equal(t, v1.OperationPut, h.Method)

		api, err := h.APIController(controller.Options{})
		require.NoError(t, err)
		_, ok := api.(*defaultoperation.DefaultAsyncPut[*rpctest.TestResourceDataModel, rpctest.TestResourceDataModel])
		require.True(t, ok)
		require.Equal(t, "Applications.Compute/virtualMachines", h.ResourceType)
		require.Equal(t, "applications.compute/virtualmachines/{virtualMachineName}", h.ResourceNamePattern)
		require.Empty(t, h.Path)
	})
}

func TestResourceOption_PatchOutput(t *testing.T) {
	node := &ResourceNode{Name: "virtualMachines", Kind: TrackedResourceKind}

	t.Run("disabled is true", func(t *testing.T) {
		option := &ResourceOption[*rpctest.TestResourceDataModel, rpctest.TestResourceDataModel]{
			linkedNode: node,
			Patch: Operation[rpctest.TestResourceDataModel]{
				Disabled: true,
			},
		}
		require.Nil(t, option.patchOutput(BuildOptions{}))
	})

	t.Run("custom controller", func(t *testing.T) {
		option := &ResourceOption[*rpctest.TestResourceDataModel, rpctest.TestResourceDataModel]{
			linkedNode: node,
			Patch: Operation[rpctest.TestResourceDataModel]{
				APIController: func(opt controller.Options) (controller.Controller, error) {
					return nil, errors.New("ok")
				},
			},
		}
		h := option.patchOutput(testBuildOptionsWithName)
		require.NotNil(t, h)
		_, err := h.APIController(controller.Options{})
		require.EqualError(t, err, "ok")
		require.Equal(t, v1.OperationPatch, h.Method)
		require.Equal(t, "Applications.Compute/virtualMachines", h.ResourceType)
		require.Equal(t, "applications.compute/virtualmachines/{virtualMachineName}", h.ResourceNamePattern)
		require.Empty(t, h.Path)
	})

	t.Run("default sync controller", func(t *testing.T) {
		option := &ResourceOption[*rpctest.TestResourceDataModel, rpctest.TestResourceDataModel]{
			linkedNode: node,
			Patch:      Operation[rpctest.TestResourceDataModel]{},
		}
		h := option.patchOutput(testBuildOptionsWithName)
		require.NotNil(t, h)
		require.Equal(t, v1.OperationPatch, h.Method)

		api, err := h.APIController(controller.Options{})
		require.NoError(t, err)
		_, ok := api.(*defaultoperation.DefaultSyncPut[*rpctest.TestResourceDataModel, rpctest.TestResourceDataModel])
		require.True(t, ok)
		require.Equal(t, "Applications.Compute/virtualMachines", h.ResourceType)
		require.Equal(t, "applications.compute/virtualmachines/{virtualMachineName}", h.ResourceNamePattern)
		require.Empty(t, h.Path)
	})

	t.Run("default async controller", func(t *testing.T) {
		option := &ResourceOption[*rpctest.TestResourceDataModel, rpctest.TestResourceDataModel]{
			linkedNode: node,
			Patch: Operation[rpctest.TestResourceDataModel]{
				AsyncJobController: func(opts asyncctrl.Options) (asyncctrl.Controller, error) {
					return nil, nil
				},
			},
		}
		h := option.patchOutput(testBuildOptionsWithName)
		require.NotNil(t, h)
		require.Equal(t, v1.OperationPatch, h.Method)

		api, err := h.APIController(controller.Options{})
		require.NoError(t, err)
		_, ok := api.(*defaultoperation.DefaultAsyncPut[*rpctest.TestResourceDataModel, rpctest.TestResourceDataModel])
		require.True(t, ok)
		require.Equal(t, "Applications.Compute/virtualMachines", h.ResourceType)
		require.Equal(t, "applications.compute/virtualmachines/{virtualMachineName}", h.ResourceNamePattern)
		require.Empty(t, h.Path)
	})
}

func TestResourceOption_DeleteOutput(t *testing.T) {
	node := &ResourceNode{Name: "virtualMachines", Kind: TrackedResourceKind}

	t.Run("disabled is true", func(t *testing.T) {
		option := &ResourceOption[*rpctest.TestResourceDataModel, rpctest.TestResourceDataModel]{
			linkedNode: node,
			Delete: Operation[rpctest.TestResourceDataModel]{
				Disabled: true,
			},
		}
		require.Nil(t, option.deleteOutput(BuildOptions{}))
	})

	t.Run("custom controller", func(t *testing.T) {
		option := &ResourceOption[*rpctest.TestResourceDataModel, rpctest.TestResourceDataModel]{
			linkedNode: node,
			Delete: Operation[rpctest.TestResourceDataModel]{
				APIController: func(opt controller.Options) (controller.Controller, error) {
					return nil, errors.New("ok")
				},
			},
		}
		h := option.deleteOutput(testBuildOptionsWithName)
		require.NotNil(t, h)
		_, err := h.APIController(controller.Options{})
		require.EqualError(t, err, "ok")
		require.Equal(t, v1.OperationDelete, h.Method)
		require.Equal(t, "Applications.Compute/virtualMachines", h.ResourceType)
		require.Equal(t, "applications.compute/virtualmachines/{virtualMachineName}", h.ResourceNamePattern)
		require.Empty(t, h.Path)
	})

	t.Run("default sync controller", func(t *testing.T) {
		option := &ResourceOption[*rpctest.TestResourceDataModel, rpctest.TestResourceDataModel]{
			linkedNode: node,
			Delete:     Operation[rpctest.TestResourceDataModel]{},
		}
		h := option.deleteOutput(testBuildOptionsWithName)
		require.NotNil(t, h)
		require.Equal(t, v1.OperationDelete, h.Method)

		api, err := h.APIController(controller.Options{})
		require.NoError(t, err)
		_, ok := api.(*defaultoperation.DefaultSyncDelete[*rpctest.TestResourceDataModel, rpctest.TestResourceDataModel])
		require.True(t, ok)
		require.Equal(t, "Applications.Compute/virtualMachines", h.ResourceType)
		require.Equal(t, "applications.compute/virtualmachines/{virtualMachineName}", h.ResourceNamePattern)
		require.Empty(t, h.Path)
	})

	t.Run("default async controller", func(t *testing.T) {
		option := &ResourceOption[*rpctest.TestResourceDataModel, rpctest.TestResourceDataModel]{
			linkedNode: node,
			Delete: Operation[rpctest.TestResourceDataModel]{
				AsyncJobController: func(opts asyncctrl.Options) (asyncctrl.Controller, error) {
					return nil, nil
				},
			},
		}
		h := option.deleteOutput(testBuildOptionsWithName)
		require.NotNil(t, h)
		require.Equal(t, v1.OperationDelete, h.Method)

		api, err := h.APIController(controller.Options{})
		require.NoError(t, err)
		_, ok := api.(*defaultoperation.DefaultAsyncDelete[*rpctest.TestResourceDataModel, rpctest.TestResourceDataModel])
		require.True(t, ok)
		require.Equal(t, "Applications.Compute/virtualMachines", h.ResourceType)
		require.Equal(t, "applications.compute/virtualmachines/{virtualMachineName}", h.ResourceNamePattern)
		require.Empty(t, h.Path)
	})
}

func TestResourceOption_CustomActionOutput(t *testing.T) {
	node := &ResourceNode{Name: "virtualMachines", Kind: TrackedResourceKind}
	t.Run("valid custom action", func(t *testing.T) {
		option := &ResourceOption[*rpctest.TestResourceDataModel, rpctest.TestResourceDataModel]{
			linkedNode: node,
			Custom: map[string]Operation[rpctest.TestResourceDataModel]{
				"start": {
					APIController: func(opt controller.Options) (controller.Controller, error) {
						return nil, nil
					},
				},
				"stop": {
					APIController: func(opt controller.Options) (controller.Controller, error) {
						return nil, nil
					},
				},
			},
		}

		hs := option.customActionOutputs(testBuildOptionsWithName)
		require.Len(t, hs, 2)

		require.NotNil(t, hs[0].APIController)
		require.NotNil(t, hs[1].APIController)

		// Reset APIController to nil for comparison
		hs[0].APIController = nil
		hs[1].APIController = nil

		require.ElementsMatch(t, []*OperationRegistration{
			{
				ResourceType:        "Applications.Compute/virtualMachines",
				ResourceNamePattern: "applications.compute/virtualmachines/{virtualMachineName}",
				Path:                "/start",
				Method:              "ACTIONSTART",
			},
			{
				ResourceType:        "Applications.Compute/virtualMachines",
				ResourceNamePattern: "applications.compute/virtualmachines/{virtualMachineName}",
				Path:                "/stop",
				Method:              "ACTIONSTOP",
			},
		}, hs)
	})

	t.Run("APIController is not defined", func(t *testing.T) {
		option := &ResourceOption[*rpctest.TestResourceDataModel, rpctest.TestResourceDataModel]{
			linkedNode: node,
			Custom: map[string]Operation[rpctest.TestResourceDataModel]{
				"start": {},
			},
		}
		require.Panics(t, func() {
			_ = option.customActionOutputs(testBuildOptionsWithName)
		})
	})
}
