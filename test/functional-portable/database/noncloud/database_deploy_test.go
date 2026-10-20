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

package database

import (
	"context"
	"fmt"
	"testing"

	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/validation"
)

// Test_DatabaseEnabled_MinimalDeploy deploys a minimal application and container and validates both
// the Radius resources and the rendered Kubernetes objects. It is the primary coverage this package
// adds: the deployment writes to the PostgreSQL store and the validation reads back through the
// resource providers' own database identities, which is exactly the path that failed when the
// per-RP users lacked privileges on the `resources` table.
//
// The suite is intentionally one small deployment. Broader deployment behaviour is already covered
// by the other functional legs; this leg only needs to prove that the same behaviour holds when the
// control plane is backed by PostgreSQL instead of the Kubernetes API server.
func Test_DatabaseEnabled_MinimalDeploy(t *testing.T) {
	template := "testdata/database-enabled-container.bicep"
	name := "database-enabled-container"

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.CoreApplicationsResource,
					},
					{
						Name: "db-ctnr",
						Type: validation.ComputeContainersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					name: {
						validation.NewK8sPodForResource(name, "db-ctnr"),
					},
				},
			},
			// The deployment above is the strongest available evidence that the control plane is
			// backed by PostgreSQL: it writes resources through applications-rp and dynamic-rp and
			// then reads them back. Asserting here, after validation and before the resources are
			// deleted, proves those writes did not land in the apiserver store.
			PostStepVerify: func(ctx context.Context, t *testing.T, ct rp.RPTest) {
				requireAPIServerStoreUnused(ctx, t, ct.Options.DynamicClient)
			},
		},
	})

	// Assert the install precondition before the deployment starts. t.Parallel() is only reached
	// inside test.Test, so this still runs sequentially and fails fast on a cluster that was
	// installed without database.enabled=true.
	requireDatabaseInstalled(t.Context(), t, test.Options.K8sClient)

	// Radius.Compute/containers is recipe-driven, so the deployment needs a preview environment,
	// which registers the default recipe pack and owns the Kubernetes namespace the container is
	// rendered into. Without this the deployment fails to resolve the container recipe.
	preSetup, previewEnvID := rp.NewPreviewEnvPreSetup(name, test.Options.Workspace.Scope, name)
	test.PreSetup = preSetup
	test.Steps[0].Executor = step.NewDeployExecutor(template, testutil.GetMagpieImage(), fmt.Sprintf("environment=%s", previewEnvID))

	test.Test(t)
}
