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

func TestInvalidArm(t *testing.T) {
	t.Parallel()

	ctx, cancel := utils.GetContext(t)
	defer cancel()

	controllerStep := kubernetestest.ControllerStep{
		Namespace:      "invalidarm",
		TemplateFolder: "testdata/invalidarm/",
	}

	test := kubernetestest.NewControllerTest(ctx, controllerStep)
	err := test.Test(t)

	require.Error(t, err)
	require.Equal(t, "admission webhook \"vcomponent.radius.dev\" denied the request: failed validation(s):\n- (root).traits.0: Additional property appId is not allowed\n", err.Error())
}
