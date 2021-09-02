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

func TestInvalidTraitDefinition(t *testing.T) {
	t.Parallel()

	ctx, cancel := testcontext.GetContext(t)
	defer cancel()

	controllerStep := kubernetestest.ControllerStep{
		Namespace:      "invalidcomponent",
		TemplateFolder: "testdata/invalidcomponent/",
	}

	test := kubernetestest.NewControllerTest(ctx, controllerStep)
	err := test.Test(t)

	require.Error(t, err)
	require.Equal(t, "admission webhook \"component-validation.radius.dev\" denied the request: failed validation(s):\n- (root).properties.traits.0: Additional property appId is not allowed\n", err.Error())
}

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
	require.Equal(t, "failed to create typed patch object: .spec.hierarchy: expected list, got &{bar}", err.Error())
}

// Our crds have very basic OpenAPI validation, which is auto generated from
// the CRD type definitions (component_types.go).
// For example, we validate the the type `kind` is a string, but not that
// the trait can only be oneof any trait type.
func TestBasicInvalid(t *testing.T) {
	t.Parallel()

	ctx, cancel := testcontext.GetContext(t)
	defer cancel()

	controllerStep := kubernetestest.ControllerStep{
		Namespace:      "invalidbadkind",
		TemplateFolder: "testdata/invalidbadkind/",
	}

	test := kubernetestest.NewControllerTest(ctx, controllerStep)
	err := test.Test(t)

	require.Error(t, err)
	require.Equal(t, "failed to create typed patch object: .spec.kind: expected string, got &value.valueUnstructured{Value:[]interface {}{}}", err.Error())
}
