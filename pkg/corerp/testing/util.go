// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package testing

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

// ReadFixture reads testdata fixtures.
func ReadFixture(filename string) []byte {
	raw, err := os.ReadFile("./testdata/" + filename)
	if err != nil {
		return nil
	}
	return raw
}

// TestContext represents the context of controller tests including common mocks.
type TestContext struct {
	Ctx    context.Context
	MCtrl  *gomock.Controller
	MockSC *store.MockStorageClient
	MockSP *dataprovider.MockDataStorageProvider
}

// NewTestContext creates new TestContext.
func NewTestContext(t *testing.T) *TestContext {
	mctrl := gomock.NewController(t)
	return &TestContext{
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
