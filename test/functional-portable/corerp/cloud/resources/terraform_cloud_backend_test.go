/*
Copyright 2026 The Radius Authors.

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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/google/uuid"
	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	"github.com/radius-project/radius/test"
	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/validation"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	cloudBackendTimeout    = 5 * time.Minute
	radiusOperationTimeout = 15 * time.Minute
)

type cloudBackendFixture struct {
	settings      map[string]any
	providers     map[string]any
	parameters    map[string]any
	prefix        string
	resourceType  string
	resourceID    func(string) string
	read          func(context.Context, string) ([]byte, error)
	keys          func(context.Context) ([]string, error)
	verifyObject  func(context.Context, string, string) (bool, error)
	cleanupObject func(context.Context, string) error
}

// These tests deliberately fail, rather than skip, when cloud prerequisites are
// missing. Both credentials are installed in the corerp-cloud CI lane.
func Test_TerraformCloudBackend_S3(t *testing.T) {
	testTerraformCloudBackend(t, "s3", newS3BackendFixture)
}

func Test_TerraformCloudBackend_AzureRM(t *testing.T) {
	testTerraformCloudBackend(t, "azurerm", newAzureBackendFixture)
}

func testTerraformCloudBackend(t *testing.T, backend string, setup func(context.Context, *testing.T, string) cloudBackendFixture) {
	t.Helper()
	name := "tfbackend-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	ct := rp.NewRPTest(t, name, nil)
	ct.FastCleanup = false
	ct.Steps = []rp.TestStep{{
		Executor: step.NewFuncExecutor(func(ctx context.Context, t *testing.T, _ test.TestOptions) {
			credential := "aws"
			if backend == "azurerm" {
				credential = "azure"
			}
			require.True(t, validation.AssertCredentialExists(t, credential), "cloud suite requires a registered default credential")
			moduleServer := requiredCloudEnv(t, "TF_RECIPE_MODULE_SERVER_URL")
			fixture := setup(ctx, t, name)

			// Every attempted Radius allocation gets a cleanup, including failed PUTs.
			// LIFO ordering keeps settings and backing storage alive through destroy.
			put := func(resourceType, resourceName string, properties map[string]any, allocate bool) generated.GenericResource {
				if allocate {
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
				}
				opCtx, cancel := context.WithTimeout(ctx, radiusOperationTimeout)
				defer cancel()
				resource, err := ct.Options.ManagementClient.CreateOrUpdateResource(opCtx, resourceType, resourceName, &generated.GenericResource{
					Location: new("global"), Properties: properties,
				})
				require.NoError(t, err, "PUT %s/%s", resourceType, resourceName)
				require.NotNil(t, resource.ID)
				return resource
			}
			destroy := func(resourceName string) {
				opCtx, cancel := context.WithTimeout(ctx, radiusOperationTimeout)
				defer cancel()
				_, err := ct.Options.ManagementClient.DeleteResource(opCtx, "Applications.Core/extenders", resourceName, false)
				require.NoError(t, err)
				_, err = ct.Options.ManagementClient.GetResource(opCtx, "Applications.Core/extenders", resourceName)
				require.True(t, azureNotFound(err), "Radius resource must be absent after synchronous destroy")
			}

			names := []string{name + "-a", name + "-b"}
			// SDK fallback cleanup is separate from assertions: it cannot make a failed
			// Terraform destroy look successful and runs before backend storage cleanup.
			for _, objectName := range names {
				t.Cleanup(func() {
					cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), cloudBackendTimeout)
					defer cancel()
					if err := fixture.cleanupObject(cleanupCtx, objectName); err != nil {
						t.Errorf("cleanup recipe object %s: %v", objectName, err)
					}
				})
			}
			config := put("Radius.Core/terraformSettings", name, map[string]any{"backend": fixture.settings}, true)
			pack := put("Radius.Core/recipePacks", name, map[string]any{
				"recipes": map[string]any{"Applications.Core/extenders": map[string]any{
					"kind": "terraform", "source": strings.TrimRight(moduleServer, "/") + "/backend-" + backend + ".zip",
				}},
			}, true)
			fixture.providers["kubernetes"] = map[string]any{"namespace": name}
			env := put("Radius.Core/environments", name, map[string]any{
				"terraformSettings": *config.ID, "recipePacks": []string{*pack.ID}, "providers": fixture.providers,
			}, true)
			app := put("Applications.Core/applications", name, map[string]any{"environment": *env.ID}, true)

			deploy := func(objectName, revision string, allocate bool) string {
				parameters := map[string]any{"name": objectName, "revision": revision}
				for key, value := range fixture.parameters {
					parameters[key] = value
				}
				resource := put("Applications.Core/extenders", objectName, map[string]any{
					"environment": *env.ID, "application": *app.ID,
					"recipe": map[string]any{"name": "default", "parameters": parameters},
				}, allocate)
				return expectedCloudStateKey(fixture.prefix, name, name, *resource.ID)
			}
			read := func(key string) cloudTerraformState {
				opCtx, cancel := context.WithTimeout(ctx, time.Minute)
				defer cancel()
				body, err := fixture.read(opCtx, key)
				require.NoError(t, err, "read expected backing-state key %s", key)
				state, err := decodeCloudState(body)
				require.NoError(t, err) // The decoder never includes state content in errors.
				require.Equal(t, 4, state.Version)
				require.NotEmpty(t, state.Lineage)
				require.Positive(t, state.Serial)
				return state
			}
			verifyObject := func(objectName, revision string) {
				pollCloud(t, ctx, "recipe object "+objectName, func(ctx context.Context) (bool, error) {
					return fixture.verifyObject(ctx, objectName, revision)
				})
			}
			verifyManaged := func(state cloudTerraformState, objectName, revision string) {
				require.Equal(t, 1, len(state.Resources))
				resource := state.Resources[0]
				require.Equal(t, "managed", resource.Mode)
				require.Equal(t, fixture.resourceType, resource.Type)
				require.Equal(t, 1, len(resource.Instances))
				require.Equal(t, fixture.resourceID(objectName), resource.Instances[0].Attributes.ID)
				require.Equal(t, revision, resource.Instances[0].Attributes.Tags["revision"])
				verifyObject(objectName, revision)
			}

			keyA := deploy(names[0], "one", true)
			first := read(keyA)
			verifyManaged(first, names[0], "one")
			keyB := deploy(names[1], "one", true)
			second := read(keyB)
			verifyManaged(second, names[1], "one")
			require.NotEqual(t, keyA, keyB)
			require.NotEqual(t, first.Lineage, second.Lineage)
			require.Equal(t, first.digest, read(keyA).digest, "creating B must not change A")

			require.Equal(t, keyA, deploy(names[0], "two", false))
			updated := read(keyA)
			require.Equal(t, first.Lineage, updated.Lineage)
			require.Greater(t, updated.Serial, first.Serial)
			verifyManaged(updated, names[0], "two")
			require.Equal(t, second.digest, read(keyB).digest, "updating A must not change B")
			verifyObject(names[1], "one")

			destroy(names[0])
			deleted := read(keyA)
			require.Equal(t, updated.Lineage, deleted.Lineage)
			require.Greater(t, deleted.Serial, updated.Serial)
			require.Zero(t, len(deleted.Resources))
			verifyObject(names[0], "")
			require.Equal(t, second.digest, read(keyB).digest, "destroying A must not change B")
			verifyObject(names[1], "one")

			destroy(names[1])
			deletedB := read(keyB)
			require.Equal(t, second.Lineage, deletedB.Lineage)
			require.Greater(t, deletedB.Serial, second.Serial)
			require.Zero(t, len(deletedB.Resources))
			verifyObject(names[1], "")
			require.Equal(t, deleted.digest, read(keyA).digest)
			opCtx, cancel := context.WithTimeout(ctx, time.Minute)
			defer cancel()
			keys, err := fixture.keys(opCtx)
			require.NoError(t, err, "backing storage must still exist after both destroys")
			require.ElementsMatch(t, []string{keyA, keyB}, keys, "state retained, no extra state or lock objects")
		}),
		SkipKubernetesOutputResourceValidation: true,
		SkipObjectValidation:                   true,
	}}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), time.Minute)
		defer cancel()
		err := ct.Options.K8sClient.CoreV1().Namespaces().Delete(ctx, name, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			t.Errorf("delete test namespace: %v", err)
		}
	})
	ct.Test(t)
}

func requiredCloudEnv(t *testing.T, key string) string {
	t.Helper()
	value := os.Getenv(key)
	require.NotEmpty(t, value, "%s is required by the cloud suite", key)
	return value
}

func azureNotFound(err error) bool {
	var response *azcore.ResponseError
	return errors.As(err, &response) && response.StatusCode == 404
}

// A timed-out PUT can still be Updating server-side. Unlike fast cleanup, do
// not force-delete its metadata while Terraform may still be running.
func deleteRadiusAfterUpdate(ctx context.Context, deleteResource func(context.Context) error) error {
	err := wait.PollUntilContextCancel(ctx, 5*time.Second, true, func(ctx context.Context) (bool, error) {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		err := deleteResource(ctx)
		if err == nil || azureNotFound(err) {
			return true, nil
		}
		var response *azcore.ResponseError
		if errors.As(err, &response) && response.StatusCode == 409 {
			return false, nil
		}
		return false, err
	})
	if err != nil {
		return fmt.Errorf("Radius cleanup failed (409 conflicts retried without force): %w", err)
	}
	return nil
}

func pollCloud(t *testing.T, ctx context.Context, description string, check func(context.Context) (bool, error)) {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		ready, err := check(ctx)
		require.NoError(t, err, description)
		if ready {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting for %s", description)
		case <-ticker.C:
		}
	}
}

// Keep the oracle independent of production backend rendering.
func expectedCloudStateKey(prefix, environment, application, resourceID string) string {
	hash := sha256.Sum256([]byte(strings.ToLower(environment + "-" + application + "-" + resourceID)))
	return prefix + "/" + hex.EncodeToString(hash[:])[:40] + ".tfstate"
}

type cloudTerraformState struct {
	Version   int    `json:"version"`
	Lineage   string `json:"lineage"`
	Serial    uint64 `json:"serial"`
	Resources []struct {
		Mode      string `json:"mode"`
		Type      string `json:"type"`
		Instances []struct {
			Attributes struct {
				ID   string            `json:"id"`
				Tags map[string]string `json:"tags"`
			} `json:"attributes"`
		} `json:"instances"`
	} `json:"resources"`
	digest [sha256.Size]byte
}

func decodeCloudState(body []byte) (cloudTerraformState, error) {
	var state cloudTerraformState
	if err := json.Unmarshal(body, &state); err != nil {
		return state, errors.New("invalid Terraform state JSON (content redacted)")
	}
	state.digest = sha256.Sum256(body)
	return state, nil
}

func readCloudStateBody(body io.ReadCloser) ([]byte, error) {
	defer body.Close()
	const limit = 1024 * 1024
	data, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return nil, errors.New("reading Terraform state failed (content redacted)")
	}
	if len(data) > limit {
		return nil, fmt.Errorf("Terraform test state exceeds %d bytes", limit)
	}
	return data, nil
}
