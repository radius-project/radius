// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kube

import (
	"encoding/json"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/ucp/store"
)

const (
	testEnvID = "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env"
	testAppID = "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/app"
)

func fakeStoreObject(dm conv.DataModelInterface) *store.Object {
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
