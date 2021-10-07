// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controllers

import (
	"os"
	"testing"

	"github.com/Azure/radius/test/kubernetestest"
	"github.com/Azure/radius/test/testcontext"
	"github.com/Azure/radius/test/validation"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	err := kubernetestest.StartController()
	if err != nil {
		panic(err)
	}
	code := m.Run()

	err = kubernetestest.StopController()
	if err != nil {
		panic(err)
	}

	os.Exit(code)
}

// Validates frontend and backend are created from arm template with content
func TestFrontendBackendArm(t *testing.T) {
	t.Parallel()
	ctx, cancel := testcontext.GetContext(t)
	defer cancel()

	controllerStep := kubernetestest.ControllerStep{
		Namespace:      "arm",
		TemplateFolder: "testdata/arm/",
		Deployments: validation.K8sObjectSet{
			Namespaces: map[string][]validation.K8sObject{
				"arm": {
					validation.NewK8sObjectForResource("kubernetes-resources-container-httpbinding", "frontend"),
					validation.NewK8sObjectForResource("kubernetes-resources-container-httpbinding", "backend"),
				},
			},
		},
	}

	test := kubernetestest.NewControllerTest(ctx, controllerStep)
	err := test.Test(t)
	require.NoError(t, err, "Test failed to start")
	test.ValidateDeploymentsRunning(t)
}
