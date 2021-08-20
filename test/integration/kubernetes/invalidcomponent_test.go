// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controllers

import (
	"testing"

	"github.com/Azure/radius/test/kubernetestest"
	"github.com/Azure/radius/test/utils"
	"github.com/stretchr/testify/require"
)

func TestInvalid(t *testing.T) {
	ctx, cancel := utils.GetContext(t)
	defer cancel()

	controllerStep := kubernetestest.ControllerStep{
		Namespace:      "invalidcomponent",
		TemplateFolder: "testdata/invalidcomponent/",
	}

	test := kubernetestest.NewControllerTest(ctx, controllerStep)
	err := test.Test(t)

	require.Error(t, err)
}
