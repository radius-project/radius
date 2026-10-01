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

package resource_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	"github.com/radius-project/radius/test"
	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/validation"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// cloudBackendAuthConflictCase describes one execution-environment variable that must never reach
// Terraform, and the registered credential whose state access it would otherwise replace.
type cloudBackendAuthConflictCase struct {
	// backend is the Terraform backend type, and credential is the cloud whose default credential
	// must be registered for the case to be meaningful.
	backend    string
	credential string

	// settings is the backend block. It deliberately names storage that does not exist; see the
	// comment on testCloudBackendRejectsStateAuthOverride.
	settings func(name string) map[string]any

	// variable is the execution-environment variable under test, and value is an obviously fake
	// placeholder. Radius rejects on the variable being set at all, so the value is never used.
	variable string
	value    string

	// terraformEvidence are substrings that appear only if Terraform actually ran against the
	// backend, which would mean the guard did not stop the execution.
	terraformEvidence []string
}

// Terraform resolves several environment variables ahead of the identity Radius renders into the
// generated backend block, so a single stray one silently authenticates state access as an
// unregistered credential. Radius rejects those execution environments.
//
// These are the native regressions for that guard: the stubbed executor used by the unit tests can
// only show Radius builds the right environment, not that a real Terraform run is prevented.
func Test_TerraformCloudBackend_AzureRM_RejectsConflictingStateAuth(t *testing.T) {
	testCloudBackendRejectsStateAuthOverride(t, cloudBackendAuthConflictCase{
		backend:    "azurerm",
		credential: "azure",
		settings: func(name string) map[string]any {
			return map[string]any{
				"type": "azurerm",
				// Valid in shape, guaranteed absent in fact: 3-24 lowercase alphanumerics.
				"storageAccountName": "tfabsent" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12],
				"containerName":      "tfstate",
				"keyPrefix":          name,
			}
		},
		// The azurerm backend checks access_key before Entra ID authentication at all, so this
		// would take over state access entirely while the rendered workload identity sat unused.
		variable:          "ARM_ACCESS_KEY",
		value:             "not-a-real-storage-account-key",
		terraformEvidence: []string{"StorageAccountNotFound", "ContainerNotFound", "AuthorizationFailure", "no such host"},
	})
}

func Test_TerraformCloudBackend_S3_RejectsStateEndpointOverride(t *testing.T) {
	testCloudBackendRejectsStateAuthOverride(t, cloudBackendAuthConflictCase{
		backend:    "s3",
		credential: "aws",
		settings: func(name string) map[string]any {
			return map[string]any{
				"type":      "s3",
				"bucket":    "radius-absent-" + strings.ReplaceAll(uuid.NewString(), "-", ""),
				"region":    "us-west-2",
				"keyPrefix": name,
			}
		},
		// An endpoint override redirects both the state traffic and the IRSA token exchange, so it
		// is rejected for every AWS credential mode rather than removed.
		variable:          "AWS_ENDPOINT_URL",
		value:             "http://127.0.0.1:1",
		terraformEvidence: []string{"NoSuchBucket", "AccessDenied", "connection refused"},
	})
}

// testCloudBackendRejectsStateAuthOverride deploys a recipe whose TerraformSettings carry the
// offending variable and asserts Radius refuses the deployment.
//
// The backend names storage that does not exist, which is what makes the assertion sharp rather
// than merely checking that something failed: Radius rejects before Terraform is ever invoked, so a
// working guard fails naming the variable, while a regressed one would reach `terraform init` and
// fail with a storage error instead. The test distinguishes the two.
//
// Settings creation itself must succeed. The variable is legitimate for recipe providers and is
// only rejected for executions that use a cloud backend, so rejecting it at PUT time would be wrong.
func testCloudBackendRejectsStateAuthOverride(t *testing.T, tc cloudBackendAuthConflictCase) {
	t.Helper()
	name := "tfauth-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	ct := rp.NewRPTest(t, name, nil)
	ct.FastCleanup = false
	ct.Steps = []rp.TestStep{{
		Executor: step.NewFuncExecutor(func(ctx context.Context, t *testing.T, _ test.TestOptions) {
			require.True(t, validation.AssertCredentialExists(t, tc.credential), "cloud suite requires a registered default %s credential; this test additionally depends on CI's registered credential kind (azure workload identity / aws irsa), because other kinds delete the variable instead of rejecting it", tc.credential)
			moduleServer := requiredCloudEnv(t, "TF_RECIPE_MODULE_SERVER_URL")

			// Register the recipe-backed resource type the guard is exercised through.
			registerCloudBackendResourceType(ctx, t, ct.Options.ConfigFilePath)

			put := func(resourceType, resourceName string, properties map[string]any) generated.GenericResource {
				t.Cleanup(func() {
					cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), cloudBackendTimeout)
					defer cancel()
					err := deleteRadiusAfterUpdate(cleanupCtx, func(ctx context.Context) error {
						_, err := ct.Options.ManagementClient.DeleteResource(ctx, resourceType, resourceName, false)
						return err
					})
					if err != nil {
						t.Errorf("cleanup Radius %s/%s: %v", resourceType, resourceName, err)
					}
				})
				opCtx, cancel := context.WithTimeout(ctx, radiusOperationTimeout)
				defer cancel()
				resource, err := ct.Options.ManagementClient.CreateOrUpdateResource(opCtx, resourceType, resourceName, &generated.GenericResource{
					Location: new("global"), Properties: properties,
				})
				require.NoError(t, err, "PUT %s/%s", resourceType, resourceName)
				require.NotNil(t, resource.ID)
				return resource
			}

			config := put("Radius.Core/terraformSettings", name, map[string]any{
				"backend": tc.settings(name),
				"env":     map[string]any{tc.variable: tc.value},
			})
			pack := put("Radius.Core/recipePacks", name, map[string]any{
				"recipes": map[string]any{cloudBackendResourceType: map[string]any{
					"kind": "terraform", "source": strings.TrimRight(moduleServer, "/") + "/backend-" + tc.backend + ".zip",
				}},
			})
			env := put("Radius.Core/environments", name, map[string]any{
				"terraformSettings": *config.ID, "recipePacks": []string{*pack.ID},
				"providers": map[string]any{"kubernetes": map[string]any{"namespace": name}},
			})
			app := put("Applications.Core/applications", name, map[string]any{"environment": *env.ID})

			// The guard also runs on the delete path, so a resource rejected at deploy time cannot
			// be deleted until the offending variable is removed from the settings. That is
			// asserted rather than worked around: it is delete-path regression coverage, and it is
			// why this test deliberately leaves the resource record behind. The guard stops Radius
			// before `terraform init`, so nothing was provisioned and the leftover is a Radius
			// record that leaks no cloud resources. The application, environment, recipe pack and
			// settings still delete normally; Radius does not block on referencing resources.
			//
			// A regression that let the deployment through would instead leave a real deployed
			// resource, so the delete must succeed in that case and is still attempted here.
			deployRejected := false
			t.Cleanup(func() {
				cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), cloudBackendTimeout)
				defer cancel()
				err := deleteRadiusAfterUpdate(cleanupCtx, func(ctx context.Context) error {
					_, err := ct.Options.ManagementClient.DeleteResource(ctx, cloudBackendResourceType, name, false)
					return err
				})
				switch {
				case err == nil:
					if deployRejected {
						t.Errorf("delete of %s/%s succeeded while %s is still set: the guard must refuse the delete path for the same reason it refused the deployment", cloudBackendResourceType, name, tc.variable)
					}
				case strings.Contains(err.Error(), tc.variable):
					// Expected: the delete is refused while the offending variable is still set.
				default:
					t.Errorf("delete %s/%s: %v", cloudBackendResourceType, name, err)
				}
			})
			opCtx, cancel := context.WithTimeout(ctx, radiusOperationTimeout)
			defer cancel()
			_, err := ct.Options.ManagementClient.CreateOrUpdateResource(opCtx, cloudBackendResourceType, name, &generated.GenericResource{
				Location: new("global"),
				Properties: map[string]any{
					"environment": *env.ID, "application": *app.ID,
					"recipe": map[string]any{"name": "default", "parameters": map[string]any{"name": name, "revision": "one"}},
				},
			})
			require.Error(t, err, "deployment must be rejected while %s is set in the execution environment", tc.variable)
			require.Contains(t, err.Error(), tc.variable, "the failure must name the offending variable so the operator can fix it")
			for _, evidence := range tc.terraformEvidence {
				require.NotContains(t, err.Error(), evidence,
					"Terraform must never run: a storage error means the execution environment was accepted")
			}
			deployRejected = true
		}),
		SkipKubernetesOutputResourceValidation: true,
		SkipObjectValidation:                   true,
	}}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), cloudBackendTimeout)
		defer cancel()
		err := ct.Options.K8sClient.CoreV1().Namespaces().Delete(ctx, name, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			t.Errorf("delete test namespace: %v", err)
		}
	})
	ct.Test(t)
}
