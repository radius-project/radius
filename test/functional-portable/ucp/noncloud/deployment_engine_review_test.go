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

package ucp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"uuid"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armdeployments"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/sdk"
	sdkclients "github.com/radius-project/radius/pkg/sdk/clients"
	ucp "github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/test/rp"
	ucptest "github.com/radius-project/radius/test/ucp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_DeploymentEngineReview(t *testing.T) {
	options := rp.NewRPTestOptions(t)
	deploymentHistory := map[string]bool{}
	group := "de-review-" + uuid.New().String()
	scope := createDeploymentReviewGroup(t, options.Connection, group, deploymentHistory)

	// Subtests are serial: the reserved-looking group name is intentionally fixed.
	t.Run("false_condition_nested_deployment", func(t *testing.T) {
		child := "skipped-" + uuid.New().String()
		_, result, err := deployReviewTemplate(t, options.Connection, group, deploymentHistory,
			readDeploymentReviewTemplate(t, "false-condition-nested-deployment.json"), map[string]any{"childName": child}, child)
		require.NoError(t, err)
		require.NotNil(t, result.Properties)
		require.Equal(t, "condition-skipped", deploymentReviewOutput(t, result.Properties.Outputs, "result"))
		require.Empty(t, result.Properties.OutputResources)

		status, body := deploymentReviewRequest(t, t.Context(), options.Connection, http.MethodGet,
			scope+"/providers/Microsoft.Resources/deployments/"+child, sdkclients.DeploymentsClientAPIVersion, nil)
		require.Equal(t, http.StatusOK, status)
		require.JSONEq(t, "null", string(body), "a missing deployment is returned as HTTP 200 with a null body")
	})

	t.Run("secure_module_output", func(t *testing.T) {
		// Preserve symbolic codegen from Bicep CLI 0.42.1 (caea9302e8), not the CI compiler.
		// Reproduce: bicep build testdata/deploymentengine/secure-output/main.bicep --outfile <temporary-file>
		for _, fixture := range []struct{ name, sha256 string }{
			{"main.bicep", "f8d7d38c812d1ee3e15c9ae6bdfaff40b0624d7ad2f809f7d1d2cc7735642ce1"},
			{"child-secure.bicep", "edb3eda135eb2aaae0e8b21a3669f7d5b674106208b872296bac55f483abbb10"},
			{"bicepconfig.json", "56f0c6684786f12b838b5670bd72abf5723e57abd28c9efc1cacb3acdb7298bd"},
			{"main.json", "9d43a449bc81aa2136eb3c5fbc286d21e296c5e01490ba08712b509c0dd710e4"},
		} {
			data, err := os.ReadFile(filepath.Join("testdata", "deploymentengine", "secure-output", fixture.name))
			require.NoError(t, err)
			require.Equal(t, fixture.sha256, fmt.Sprintf("%x", sha256.Sum256(data)), fixture.name)
			t.Logf("Bicep 0.42.1 fixture %s sha256=%s", fixture.name, fixture.sha256)
		}

		// The parent expression reads the same child through listOutputsWithSecureValues
		// after its declared write. A duplicate public alias makes this deployment fail.
		_, result, err := deployReviewTemplate(t, options.Connection, group, deploymentHistory,
			readDeploymentReviewTemplate(t, "secure-output/main.json"), nil, "securechild")
		require.NoError(t, err)
		require.NotNil(t, result.Properties)
		require.Equal(t, float64(10), deploymentReviewOutput(t, result.Properties.Outputs, "len"))
	})

	t.Run("resource_group_named_deployments", func(t *testing.T) {
		// Requires the isolated UCP CI job, with no concurrent creator of this
		// fixed name. HTTP preconditions are not an atomic create-if-absent lock.
		createDeploymentReviewGroup(t, options.Connection, "deployments", deploymentHistory)
		for _, spelling := range []string{"deployments", "DePlOyMeNtS"} {
			t.Run(spelling, func(t *testing.T) {
				child := "ordinary-" + uuid.New().String()
				_, result, err := deployReviewTemplate(t, options.Connection, spelling, deploymentHistory,
					readDeploymentReviewTemplate(t, "nested-output.json"), map[string]any{"childName": child}, child)
				require.NoError(t, err)
				require.NotNil(t, result.Properties)
				require.Equal(t, "ordinary-child-output", deploymentReviewOutput(t, result.Properties.Outputs, "result"))
			})
		}
	})

	t.Run("schema_validation_business_code_target", func(t *testing.T) {
		name := "invalid-" + uuid.New().String()
		id := scope + "/providers/Radius.Core/recipePacks/" + name
		cleanupDeploymentReviewResources(t, options, id)

		status, body := deploymentReviewRequest(t, t.Context(), options.Connection, http.MethodPut,
			id, "2025-08-01-preview", map[string]any{
				"location": "global",
				"properties": map[string]any{
					"recipes": 17,
				},
			})
		require.Equal(t, http.StatusBadRequest, status, "expected a real schema HTTP 400: %s", body)
		var providerError v1.ErrorResponse
		require.NoError(t, json.Unmarshal(body, &providerError))
		assertDeploymentReviewSchemaError(t, providerError.Error)

		deploymentID, _, err := deployReviewTemplate(t, options.Connection, group, deploymentHistory,
			readDeploymentReviewTemplate(t, "invalid-recipepack.json"), map[string]any{"name": name})
		require.Error(t, err, "the invalid resource must fail deployment")

		status, body = deploymentReviewRequest(t, t.Context(), options.Connection, http.MethodGet,
			deploymentID, sdkclients.DeploymentsClientAPIVersion, nil)
		require.Equal(t, http.StatusOK, status, "failed deployment must remain readable: %s", body)
		t.Logf("Raw schema deployment error: %s", body)
		var deployment struct {
			Properties struct {
				ProvisioningState string           `json:"provisioningState"`
				Error             *v1.ErrorDetails `json:"error"`
			} `json:"properties"`
		}
		require.NoError(t, json.Unmarshal(body, &deployment))
		require.Equal(t, "Failed", deployment.Properties.ProvisioningState)
		require.NotNil(t, deployment.Properties.Error)
		require.Equal(t, "DeploymentFailed", deployment.Properties.Error.Code)
		require.Len(t, deployment.Properties.Error.Details, 1, "do not append an opaque duplicate of the provider error")
		actual := deployment.Properties.Error.Details[0]
		assertDeploymentReviewSchemaError(t, actual)
		require.Equal(t, providerError.Error.Message, actual.Message)
		require.Equal(t, providerError.Error.Details, actual.Details, "preserve the original structured property errors")
	})

	t.Run("provider_config_precedence", func(t *testing.T) {
		require.NotEmpty(t, options.Workspace.Environment, "the legacy manual extender uses the test workspace environment")
		inlineGroup := "de-inline-" + uuid.New().String()
		callerGroup := "de-caller-" + uuid.New().String()
		inlineScope := createDeploymentReviewGroup(t, options.Connection, inlineGroup, deploymentHistory)
		callerScope := createDeploymentReviewGroup(t, options.Connection, callerGroup, deploymentHistory)
		name := "precedence-" + uuid.New().String()
		resourceTypes := []struct{ name, apiVersion string }{
			{"Applications.Core/extenders", "2023-10-01-preview"},
			{"Radius.Core/recipePacks", "2025-08-01-preview"},
		}
		for _, resourceType := range resourceTypes {
			cleanupDeploymentReviewResources(t, options,
				inlineScope+"/providers/"+resourceType.name+"/"+name,
				callerScope+"/providers/"+resourceType.name+"/"+name)
		}

		template := readDeploymentReviewTemplate(t, "providerconfig-precedence.json")
		imports, ok := template["imports"].(map[string]any)
		require.True(t, ok)
		radius, ok := imports["radius"].(map[string]any)
		require.True(t, ok)
		inlineConfig, ok := radius["config"].(map[string]any)
		require.True(t, ok)
		inlineConfig["scope"] = inlineScope
		t.Logf("Inline radius scope=%s; caller radius/deployments scope=%s", inlineScope, callerScope)
		deploymentID, _, err := deployReviewTemplate(t, options.Connection, callerGroup, deploymentHistory, template,
			map[string]any{
				"name": name, "environment": options.Workspace.Environment,
			})
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(deploymentID, callerScope+"/providers/Microsoft.Resources/deployments/"))
		for _, resourceType := range resourceTypes {
			expectedID := callerScope + "/providers/" + resourceType.name + "/" + name
			status, body := deploymentReviewRequest(t, t.Context(), options.Connection, http.MethodGet,
				expectedID, resourceType.apiVersion, nil)
			require.Equal(t, http.StatusOK, status, "caller scope must own %s: %s", resourceType.name, body)
			var resource struct {
				ID string `json:"id"`
			}
			require.NoError(t, json.Unmarshal(body, &resource))
			require.True(t, strings.EqualFold(expectedID, resource.ID), "unexpected resource ID: %s", body)

			status, body = deploymentReviewRequest(t, t.Context(), options.Connection, http.MethodGet,
				inlineScope+"/providers/"+resourceType.name+"/"+name, resourceType.apiVersion, nil)
			require.Equal(t, http.StatusNotFound, status, "inline scope must not own %s: %s", resourceType.name, body)
		}
	})

	t.Run("legacy_dependency_projection", func(t *testing.T) {
		require.NotEmpty(t, options.Workspace.Environment, "the legacy manual extenders use the test workspace environment")
		name := "dependency-" + uuid.New().String()
		predecessorID := scope + "/providers/Applications.Core/extenders/" + name + "-first"
		legacyID := scope + "/providers/Applications.Core/extenders/" + name + "-second"
		modernID := scope + "/providers/Radius.Core/recipePacks/" + name
		cleanupDeploymentReviewResources(t, options, modernID, legacyID, predecessorID)

		deploymentID, _, err := deployReviewTemplate(t, options.Connection, group, deploymentHistory,
			readDeploymentReviewTemplate(t, "mixed-dependencies.json"), map[string]any{"name": name, "environment": options.Workspace.Environment})
		require.NoError(t, err)
		status, body := deploymentReviewRequest(t, t.Context(), options.Connection, http.MethodGet,
			deploymentID, sdkclients.DeploymentsClientAPIVersion, nil)
		require.Equal(t, http.StatusOK, status)
		t.Logf("Raw public deployment dependencies: %s", body)
		var deployment struct {
			Properties struct {
				Dependencies []struct {
					ID           string `json:"id"`
					SymbolicName string `json:"symbolicName"`
					DependsOn    []struct {
						ID           string `json:"id"`
						SymbolicName string `json:"symbolicName"`
					} `json:"dependsOn"`
				} `json:"dependencies"`
			} `json:"properties"`
		}
		require.NoError(t, json.Unmarshal(body, &deployment))
		expected := map[string]string{"legacyDependent": legacyID, "modernDependent": modernID}
		seen := map[string]bool{}
		for _, dependency := range deployment.Properties.Dependencies {
			if len(dependency.DependsOn) == 0 {
				continue
			}
			expectedID, ok := expected[dependency.SymbolicName]
			require.True(t, ok, "unexpected dependency identity: %+v", dependency)
			require.False(t, seen[dependency.SymbolicName], "duplicate dependency identity")
			seen[dependency.SymbolicName] = true
			require.True(t, strings.EqualFold(expectedID, dependency.ID), "dependency must have a canonical UCP ID: %+v", dependency)
			require.Len(t, dependency.DependsOn, 1)
			require.Equal(t, "legacyPredecessor", dependency.DependsOn[0].SymbolicName)
			require.True(t, strings.EqualFold(predecessorID, dependency.DependsOn[0].ID),
				"legacy predecessor must retain its canonical UCP ID: %+v", dependency)
		}
		require.Len(t, seen, len(expected), "both modern and legacy successors must expose their dependency")
	})

	t.Run("content_link_query_redaction", func(t *testing.T) {
		// These are inert markers, not credentials. Only UCP is contacted by the test;
		// the deployment engine must reject this loopback link before fetching it.
		markers := []string{"review-user-dummy", "review-password-dummy", "review-dummy-not-a-secret", "review-fragment-dummy"}
		link := fmt.Sprintf("http://%s:%s@127.0.0.1/parameters.json?sig=%s#%s", markers[0], markers[1], markers[2], markers[3])
		deploymentID := scope + "/providers/Microsoft.Resources/deployments/redaction-" + uuid.New().String()
		deploymentHistory[strings.ToLower(deploymentID)] = true
		status, body := deploymentReviewRequest(t, t.Context(), options.Connection, http.MethodPut,
			deploymentID, sdkclients.DeploymentsClientAPIVersion, sdkclients.Deployment{
				Properties: &sdkclients.DeploymentProperties{
					Mode:           armdeployments.DeploymentModeIncremental,
					ProviderConfig: sdkclients.NewDefaultProviderConfig(group),
					Template: map[string]any{
						"$schema":        "https://schema.management.azure.com/schemas/2019-04-01/deploymentTemplate.json#",
						"contentVersion": "1.0.0.0",
						"resources":      []any{},
					},
					ParametersLink: &armdeployments.ParametersLink{URI: &link},
				},
			})
		// Do not print a possibly unredacted response into JUnit on failure.
		for i, marker := range markers {
			require.False(t, strings.Contains(string(body), marker), "response leaked content-link component %d", i)
		}
		require.Equal(t, http.StatusBadRequest, status)
		var response v1.ErrorResponse
		require.NoError(t, json.Unmarshal(body, &response))
		require.NotNil(t, response.Error)
		require.Equal(t, "InvalidContentLink", response.Error.Code)
		require.Contains(t, response.Error.Message, "http://127.0.0.1/parameters.json", "retain the useful, non-secret link path")
		require.Contains(t, response.Error.Message, "invalid or not supported")
	})
}

func createDeploymentReviewGroup(t *testing.T, connection sdk.Connection, name string, deploymentHistory map[string]bool) string {
	t.Helper()
	groupClient, err := ucp.NewResourceGroupsClient(&aztoken.AnonymousCredential{}, sdk.NewClientOptions(connection))
	require.NoError(t, err)
	_, err = groupClient.Get(t.Context(), "local", name, nil)
	require.Error(t, err, "group %q already exists; refusing to modify or delete an unowned group", name)
	require.True(t, clients.Is404Error(err), "looking up group %q: %v", name, err)

	owner := uuid.New().String()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), time.Minute)
		defer cancel()
		var response *http.Response
		group, err := groupClient.Get(policy.WithCaptureResponse(ctx, &response), "local", name, nil)
		if clients.Is404Error(err) {
			return
		}
		require.NoError(t, err)
		require.Equal(t, &owner, group.Tags["deployment-engine-review"], "refusing to delete a group no longer owned by this test")
		require.NotNil(t, response)
		etag := response.Header.Get("ETag")
		require.NotEmpty(t, etag)
		resourceClient, err := ucp.NewResourcesClient(&aztoken.AnonymousCredential{}, sdk.NewClientOptions(connection))
		require.NoError(t, err)
		var retainedHistory []string
		require.EventuallyWithT(t, func(c *assert.CollectT) {
			var remaining []string
			pager := resourceClient.NewListPager("local", name, nil)
			for pager.More() {
				page, err := pager.NextPage(ctx)
				require.NoError(c, err)
				for _, resource := range page.Value {
					require.NotNil(c, resource)
					require.NotNil(c, resource.ID)
					require.True(c, deploymentHistory[strings.ToLower(*resource.ID)],
						"resource %s remains; refusing to delete the group", *resource.ID)
					remaining = append(remaining, *resource.ID)
				}
			}
			retainedHistory = remaining
		}, 30*time.Second, time.Second)

		// DE has no public deployment DELETE. Its history has a ten-minute idle
		// retention window, but UCP tracking rows can outlive it. Leave that known
		// metadata and its group for the existing test-environment teardown.
		if len(retainedHistory) > 0 {
			t.Logf("Retaining owned group %s with only this test's deployment-history IDs (no deployment DELETE API): %v", name, retainedHistory)
			return
		}

		// UCP enforces the HTTP precondition before deletion, not a storage-level
		// transaction spanning the ownership read, resource listing, and delete.
		ctx = policy.WithHTTPHeader(ctx, http.Header{"If-Match": []string{etag}})
		_, err = groupClient.Delete(ctx, "local", name, nil)
		require.NoError(t, err)
	})

	ctx := policy.WithHTTPHeader(t.Context(), http.Header{"If-None-Match": []string{"*"}})
	_, err = groupClient.CreateOrUpdate(ctx, "local", name, ucp.ResourceGroupResource{
		Location: new("global"),
		Tags:     map[string]*string{"deployment-engine-review": &owner},
	}, nil)
	require.NoError(t, err)
	return "/planes/radius/local/resourceGroups/" + name
}

func readDeploymentReviewTemplate(t *testing.T, fixture string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "deploymentengine", fixture))
	require.NoError(t, err)
	var template map[string]any
	require.NoError(t, json.Unmarshal(data, &template))
	return template
}

func deployReviewTemplate(t *testing.T, connection sdk.Connection, group string, deploymentHistory map[string]bool, template map[string]any, parameters map[string]any, children ...string) (string, sdkclients.ClientCreateOrUpdateResponse, error) {
	t.Helper()
	wrappedParameters := clients.DeploymentParameters{}
	for name, value := range parameters {
		wrappedParameters[name] = map[string]any{"value": value}
	}
	scope := "/planes/radius/local/resourceGroups/" + group
	id := scope + "/providers/Microsoft.Resources/deployments/review-" + uuid.New().String()
	for _, child := range children {
		deploymentHistory[strings.ToLower(scope+"/providers/Microsoft.Resources/deployments/"+child)] = true
	}
	deploymentHistory[strings.ToLower(id)] = true
	client, err := sdkclients.NewResourceDeploymentsClient(&sdkclients.Options{
		Cred: &aztoken.AnonymousCredential{}, BaseURI: connection.Endpoint(), ARMClientOptions: sdk.NewClientOptions(connection),
	})
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	poller, err := client.CreateOrUpdate(ctx, sdkclients.Deployment{
		Properties: &sdkclients.DeploymentProperties{
			Mode:           armdeployments.DeploymentModeIncremental,
			ProviderConfig: sdkclients.NewDefaultProviderConfig(group),
			Template:       template,
			Parameters:     wrappedParameters,
		},
	}, id, sdkclients.DeploymentsClientAPIVersion)
	if err != nil {
		return id, sdkclients.ClientCreateOrUpdateResponse{}, err
	}
	result, err := poller.PollUntilDone(ctx, &sdkclients.PollUntilDoneOptions{Frequency: time.Second})
	return id, result, err
}

func cleanupDeploymentReviewResources(t *testing.T, options rp.RPTestOptions, ids ...string) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), time.Minute)
		defer cancel()
		for _, id := range ids {
			parsed, err := resources.ParseResource(id)
			require.NoError(t, err)
			_, err = options.ManagementClient.DeleteResource(ctx, parsed.Type(), id, false)
			if clients.Is404Error(err) {
				continue
			}
			require.NoError(t, err, "deleting test-owned resource %s", id)
		}
	})
}

func deploymentReviewRequest(t *testing.T, ctx context.Context, connection sdk.Connection, method, id, apiVersion string, body any) (int, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(data)
	}
	request, err := ucptest.NewUCPRequest(method,
		sdkclients.DeploymentEngineURL(connection.Endpoint(), id)+"?api-version="+url.QueryEscape(apiVersion), reader)
	require.NoError(t, err)
	response, err := connection.Client().Do(request.WithContext(ctx))
	require.NoError(t, err)
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	return response.StatusCode, data
}

func deploymentReviewOutput(t *testing.T, outputs any, name string) any {
	t.Helper()
	data, err := json.Marshal(outputs)
	require.NoError(t, err)
	var values map[string]struct {
		Value any `json:"value"`
	}
	require.NoError(t, json.Unmarshal(data, &values))
	require.Contains(t, values, name)
	return values[name].Value
}

func assertDeploymentReviewSchemaError(t *testing.T, detail *v1.ErrorDetails) {
	t.Helper()
	require.NotNil(t, detail)
	require.Equal(t, "HttpRequestPayloadAPISpecValidationFailed", detail.Code)
	require.True(t, strings.EqualFold("Radius.Core/recipePacks", detail.Target), "preserve the qualified type target: %+v", detail)
	require.Len(t, detail.Details, 1)
	require.NotNil(t, detail.Details[0])
	require.Equal(t, "InvalidProperties", detail.Details[0].Code)
	require.Contains(t, detail.Details[0].Message, "properties.recipes")
	require.Contains(t, detail.Details[0].Message, "object")
	require.Empty(t, detail.Details[0].Details, "unexpected extra provider error branch")
}
