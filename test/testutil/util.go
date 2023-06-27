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

package testutil

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/store"
)

// MustGetTestData reads testdata and unmarshals it to the given type.
func MustGetTestData[T any](file string) *T {
	var data T
	err := json.Unmarshal(ReadFixture(file), &data)
	if err != nil {
		panic(err)
	}
	return &data
}

// ReadFixture reads testdata fixtures.
func ReadFixture(filename string) []byte {
	raw, err := os.ReadFile("./testdata/" + filename)
	if err != nil {
		panic(err)
	}
	return raw
}

// ControllerTestContext represents the context of controller tests including common mocks.
type ControllerTestContext struct {
	Ctx    context.Context
	MCtrl  *gomock.Controller
	MockSC *store.MockStorageClient
	MockSP *dataprovider.MockDataStorageProvider
}

// NewControllerTestContext creates a new ControllerTestContext for testing.
func NewControllerTestContext(t *testing.T) *ControllerTestContext {
	mctrl := gomock.NewController(t)
	return &ControllerTestContext{
		Ctx:    context.Background(),
		MCtrl:  mctrl,
		MockSC: store.NewMockStorageClient(mctrl),
		MockSP: dataprovider.NewMockDataStorageProvider(mctrl),
	}
}

// FakeStoreObject creates store.Object for datamodel.
func FakeStoreObject(dm v1.DataModelInterface) *store.Object {
	b, err := json.Marshal(dm)
	if err != nil {
		return nil
	}
	var r any
	err = json.Unmarshal(b, &r)
	if err != nil {
		return nil
	}
	return &store.Object{Data: r}
}
