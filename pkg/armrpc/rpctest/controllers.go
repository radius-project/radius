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

package rpctest

import (
	"context"
	"encoding/json"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/ucp/database"
	"go.uber.org/mock/gomock"
)

// ControllerContext represents the context of controller tests including common mocks.
type ControllerContext struct {
	Ctx    context.Context
	MCtrl  *gomock.Controller
	MockSC *database.MockClient
}

// NewControllerContext creates a new ControllerContext for testing.
func NewControllerContext(t *testing.T) *ControllerContext {
	mctrl := gomock.NewController(t)

	return &ControllerContext{
		Ctx:    context.Background(),
		MCtrl:  mctrl,
		MockSC: database.NewMockClient(mctrl),
	}
}

// FakeStoreObject creates store.Object for datamodel.
func FakeStoreObject(dm v1.DataModelInterface) *database.Object {
	b, err := json.Marshal(dm)
	if err != nil {
		panic(err)
	}
	var r any
	err = json.Unmarshal(b, &r)
	if err != nil {
		panic(err)
	}
	return &database.Object{Data: r}
}
