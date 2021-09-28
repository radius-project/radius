// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controllers

import (
	"testing"

	"github.com/Azure/radius/test/kubernetestest"
	"github.com/Azure/radius/test/testcontext"
	"github.com/stretchr/testify/require"
)

func TestInvalidApplication(t *testing.T) {
	t.Parallel()

	ctx, cancel := testcontext.GetContext(t)
	defer cancel()

	controllerStep := kubernetestest.ControllerStep{
		Namespace:      "invalidapplication",
		TemplateFolder: "testdata/invalidapplication/",
	}

	test := kubernetestest.NewControllerTest(ctx, controllerStep)
	err := test.Test(t)

	require.Error(t, err)
	require.Equal(t, "failed to create typed patch object: .spec.application: expected string, got &value.valueUnstructured{Value:123}", err.Error())
}

func TestInvalidHttpRoute(t *testing.T) {
	t.Parallel()

	ctx, cancel := testcontext.GetContext(t)
	defer cancel()

	controllerStep := kubernetestest.ControllerStep{
		Namespace:      "arm",
		TemplateFolder: "testdata/invalidhttproute/",
	}

	test := kubernetestest.NewControllerTest(ctx, controllerStep)
	err := test.Test(t)

	require.Error(t, err)
	require.Equal(t, "admission webhook \"resource-validation.radius.dev\" denied the request: failed validation(s):\n- (root).properties.gateway: Invalid type. Expected: object, given: string\n", err.Error())
}

func TestInvalidContainerComponent(t *testing.T) {
	t.Parallel()

	ctx, cancel := testcontext.GetContext(t)
	defer cancel()

	controllerStep := kubernetestest.ControllerStep{
		Namespace:      "arm",
		TemplateFolder: "testdata/invalidcontainercomponent/",
	}

	test := kubernetestest.NewControllerTest(ctx, controllerStep)
	err := test.Test(t)

	require.Error(t, err)
	require.Equal(t, "admission webhook \"resource-validation.radius.dev\" denied the request: failed validation(s):\n- (root).properties.container: Invalid type. Expected: object, given: string\n", err.Error())
}
