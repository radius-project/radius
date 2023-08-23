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

package resourcetypeutil

import (
	"encoding/json"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
)

// FakeResource is a fake resource type.
type FakeResource struct{}

// Always returns "FakeResource" as the name.
func (f *FakeResource) ResourceTypeName() string {
	return "FakeResource"
}

func (f *FakeResource) GetSystemData() *v1.SystemData {
	return nil
}

func (f *FakeResource) GetBaseResource() *v1.BaseResource {
	return nil
}

func (f *FakeResource) ProvisioningState() v1.ProvisioningState {
	return v1.ProvisioningStateAccepted
}

func (f *FakeResource) SetProvisioningState(state v1.ProvisioningState) {
}

func (f *FakeResource) UpdateMetadata(ctx *v1.ARMRequestContext, oldResource *v1.BaseResource) {
}

// MustPopulateResourceStatus populates a ResourceStatus object with an output resource commonly used in our
// test fixtures.
//
// Example usage (in a converter test):
//
//	..ResourceStatus: resourcetypeutil.MustPopulateResourceStatus(&myapiversion.ResourceStatus{})
func MustPopulateResourceStatus[T any](obj T) T {
	data := map[string]any{
		"outputResources": []map[string]any{
			{
				"id": "/planes/test/local/providers/Test.Namespace/testResources/test-resource",
			},
		},
	}

	b, err := json.Marshal(data)
	if err != nil {
		panic(err)
	}

	err = json.Unmarshal(b, obj)
	if err != nil {
		panic(err)
	}

	return obj
}
