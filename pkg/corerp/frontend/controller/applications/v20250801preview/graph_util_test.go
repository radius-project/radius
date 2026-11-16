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

package v20250801preview

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	corerpv20250801preview "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The end-to-end filter policy (excluded types, Connection-wins, direction and
// kind validation, dedupe) is exhaustively covered in
// pkg/graph/edges/edges_test.go. The tests here only assert the server-side
// plumbing:
//
//  1. computeGraph forwards dependsOnEdges to edges.MergeDependencyEdges.
//  2. edges.ExcludedResourceTypes drops caller-supplied edges from application
//     and environment sources.
//  3. Missing / nil dependsOnEdges is a no-op.
func Test_computeGraph_MergesDependsOnEdges(t *testing.T) {
	const (
		containerID = "/planes/radius/local/resourceGroups/default/providers/Radius.Compute/containers/consumer"
		queueID     = "/planes/radius/local/resourceGroups/default/providers/Radius.Messaging/rabbitMQQueues/queue"
		appID       = "/planes/radius/local/resourceGroups/default/providers/Radius.Core/applications/myapp"
	)

	container := generated.GenericResource{
		ID:   to.Ptr(containerID),
		Name: to.Ptr("consumer"),
		Type: to.Ptr("Radius.Compute/containers"),
		Properties: map[string]any{
			"application": appID,
		},
	}
	queue := generated.GenericResource{
		ID:   to.Ptr(queueID),
		Name: to.Ptr("queue"),
		Type: to.Ptr("Radius.Messaging/rabbitMQQueues"),
		Properties: map[string]any{
			"application": appID,
		},
	}
	appResource := generated.GenericResource{
		ID:   to.Ptr(appID),
		Name: to.Ptr("myapp"),
		Type: to.Ptr("Radius.Core/applications"),
	}

	dependsOnEdge := &corerpv20250801preview.ApplicationGraphConnection{
		ID:        to.Ptr(queueID),
		Direction: to.Ptr(corerpv20250801preview.DirectionOutbound),
		Kind:      to.Ptr(corerpv20250801preview.ConnectionKindDependency),
	}

	findResource := func(t *testing.T, graph *corerpv20250801preview.ApplicationGraphResponse, id string) *corerpv20250801preview.ApplicationGraphResource {
		t.Helper()
		for _, r := range graph.Resources {
			if r != nil && r.ID != nil && *r.ID == id {
				return r
			}
		}
		require.Failf(t, "resource not in graph", "id=%s", id)
		return nil
	}

	t.Run("dependsOnEdges are merged", func(t *testing.T) {
		graph := computeGraph(
			[]generated.GenericResource{container, queue},
			nil,
			"",
			map[string][]*corerpv20250801preview.ApplicationGraphConnection{
				containerID: {dependsOnEdge},
			},
		)

		// Container gains an outbound Dependency edge to queue.
		containerNode := findResource(t, graph, containerID)
		var haveOutboundDep bool
		for _, c := range containerNode.Connections {
			if c == nil || c.ID == nil || c.Direction == nil || c.Kind == nil {
				continue
			}
			if *c.ID == queueID &&
				*c.Direction == corerpv20250801preview.DirectionOutbound &&
				*c.Kind == corerpv20250801preview.ConnectionKindDependency {
				haveOutboundDep = true
			}
		}
		assert.True(t, haveOutboundDep, "container should carry outbound Dependency edge to queue")

		// Queue gains a mirrored inbound Dependency edge from container.
		queueNode := findResource(t, graph, queueID)
		var haveInboundDep bool
		for _, c := range queueNode.Connections {
			if c == nil || c.ID == nil || c.Direction == nil || c.Kind == nil {
				continue
			}
			if *c.ID == containerID &&
				*c.Direction == corerpv20250801preview.DirectionInbound &&
				*c.Kind == corerpv20250801preview.ConnectionKindDependency {
				haveInboundDep = true
			}
		}
		assert.True(t, haveInboundDep, "queue should carry mirrored inbound Dependency edge from container")
	})

	t.Run("nil dependsOnEdges leaves graph unchanged", func(t *testing.T) {
		graph := computeGraph(
			[]generated.GenericResource{container, queue},
			nil,
			"",
			nil,
		)
		containerNode := findResource(t, graph, containerID)
		for _, c := range containerNode.Connections {
			if c != nil && c.Kind != nil {
				assert.NotEqual(t, corerpv20250801preview.ConnectionKindDependency, *c.Kind,
					"no Dependency edges should be present when dependsOnEdges is nil")
			}
		}
	})

	t.Run("edges sourced from excluded types are dropped", func(t *testing.T) {
		// Radius.Core/applications is in edges.ExcludedResourceTypes, so a
		// caller-supplied edge sourced from it must not land on the graph.
		graph := computeGraph(
			[]generated.GenericResource{container, queue, appResource},
			nil,
			"",
			map[string][]*corerpv20250801preview.ApplicationGraphConnection{
				appID: {{
					ID:        to.Ptr(queueID),
					Direction: to.Ptr(corerpv20250801preview.DirectionOutbound),
					Kind:      to.Ptr(corerpv20250801preview.ConnectionKindDependency),
				}},
			},
		)
		// Queue should not carry an inbound Dependency edge from appID.
		queueNode := findResource(t, graph, queueID)
		for _, c := range queueNode.Connections {
			if c == nil || c.ID == nil {
				continue
			}
			assert.NotEqual(t, appID, *c.ID, "excluded-source Dependency edge should not be merged")
		}
	})
}

func Test_computeGraph_OmitsContainerEnvironment(t *testing.T) {
	t.Parallel()

	const (
		containerID = "/planes/radius/local/resourceGroups/default/providers/Radius.Compute/containers/frontend"
		appID       = "/planes/radius/local/resourceGroups/default/providers/Radius.Core/applications/myapp"
	)
	tests := []struct {
		name        string
		environment any
		credential  string
	}{
		{name: "plaintext", environment: map[string]any{"PASSWORD": "plaintext-credential"}, credential: "plaintext-credential"},
		{name: "secure derived", environment: map[string]any{"PASSWORD": "[parameters('password')]"}, credential: "[parameters('password')]"},
		{name: "null", environment: nil},
		{name: "empty", environment: map[string]any{}},
		{name: "unexpected shape", environment: "malformed-credential", credential: "malformed-credential"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			container := map[string]any{
				"image": "frontend:latest",
				"env":   tt.environment,
			}
			graph := computeGraph([]generated.GenericResource{{
				ID:   to.Ptr(containerID),
				Name: to.Ptr("frontend"),
				Type: to.Ptr("Radius.Compute/containers"),
				Properties: map[string]any{
					"application": appID,
					"containers":  map[string]any{"frontend": container},
				},
			}}, nil, "", nil)

			require.Len(t, graph.Resources, 1)
			payload, err := json.Marshal(graph)
			require.NoError(t, err)
			require.NotContains(t, string(payload), `"env"`)
			if tt.credential != "" {
				require.NotContains(t, string(payload), tt.credential)
			}
			require.Contains(t, container, "env")
		})
	}
}

func Test_computeGraph_OutputResourcePortalURLs(t *testing.T) {
	t.Parallel()

	const (
		tenantID       = "11111111-1111-1111-1111-111111111111"
		appID          = "/planes/radius/local/resourceGroups/default/providers/Radius.Core/applications/myapp"
		databaseID     = "/planes/radius/local/resourceGroups/default/providers/Radius.Data/mySqlDatabases/db"
		azureStorageID = "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg/providers/Microsoft.Storage/storageAccounts/mystorage"
		awsDatabaseID  = "/planes/aws/aws/accounts/123456789012/regions/us-west-2/providers/AWS.RDS/DBInstance/mydb"
		awsGovQueueID  = "/planes/aws/aws-us-gov/accounts/123456789012/regions/us-gov-west-1/providers/AWS.SQS/Queue/myqueue"
		awsUnknownID   = "/planes/aws/aws/accounts/123456789012/regions/us-west-2/providers/AWS.Unknown/Thing/mything"
		tfBucketID     = "/planes/aws/aws/accounts/123456789012/regions/global/providers/Terraform.AWS/aws_s3_bucket/mybucket"
		awsChinaRDSID  = "/planes/aws/aws-cn/accounts/123456789012/regions/cn-north-1/providers/AWS.RDS/DBInstance/mychinadb"
		tfIAMRoleID    = "/planes/aws/aws/accounts/123456789012/regions/global/providers/Terraform.AWS/aws_iam_role/myrole"
		tfSubnetID     = "/planes/aws/aws/accounts/123456789012/regions/us-west-2/providers/Terraform.AWS/aws_subnet/mysubnet"
		tfFunctionID   = "/planes/aws/aws/accounts/123456789012/regions/us-west-2/providers/Terraform.AWS/aws_lambda_function/myfunction"
		kubernetesID   = "/planes/kubernetes/local/namespaces/default/providers/apps/Deployment/mydeployment"
	)

	// The ARNs that recipe drivers store as each output resource's providerResourceId.
	arns := map[string]string{
		tfFunctionID:  "arn:aws:lambda:us-west-2:123456789012:function:myfunction",
		awsGovQueueID: "arn:aws-us-gov:sqs:us-gov-west-1:123456789012:myqueue",
		awsChinaRDSID: "arn:aws-cn:rds:cn-north-1:123456789012:db:mychinadb",
	}
	const (
		functionURL = "https://console.aws.amazon.com/go/view?arn=arn%3Aaws%3Alambda%3Aus-west-2%3A123456789012%3Afunction%3Amyfunction"
		govQueueURL = "https://console.amazonaws-us-gov.com/go/view?arn=arn%3Aaws-us-gov%3Asqs%3Aus-gov-west-1%3A123456789012%3Amyqueue"
		chinaRDSURL = "https://console.amazonaws.cn/go/view?arn=arn%3Aaws-cn%3Ards%3Acn-north-1%3A123456789012%3Adb%3Amychinadb"
	)

	tests := []struct {
		name     string
		tenantID string
		want     map[string]string
	}{
		{
			name:     "with an Azure tenant",
			tenantID: tenantID,
			want: map[string]string{
				azureStorageID: "https://portal.azure.com/#@" + tenantID + "/resource" + azureStorageID,
				awsDatabaseID:  "https://us-west-2.console.aws.amazon.com/rds/home?region=us-west-2#databases:",
				awsGovQueueID:  govQueueURL,
				awsUnknownID:   "",
				tfBucketID:     "https://console.aws.amazon.com/s3/buckets",
				awsChinaRDSID:  chinaRDSURL,
				tfIAMRoleID:    "https://console.aws.amazon.com/iam/home#/roles",
				tfSubnetID:     "",
				tfFunctionID:   functionURL,
				kubernetesID:   "",
			},
		},
		{
			name: "without an Azure tenant",
			want: map[string]string{
				azureStorageID: "",
				awsDatabaseID:  "https://us-west-2.console.aws.amazon.com/rds/home?region=us-west-2#databases:",
				awsGovQueueID:  govQueueURL,
				awsUnknownID:   "",
				tfBucketID:     "https://console.aws.amazon.com/s3/buckets",
				awsChinaRDSID:  chinaRDSURL,
				tfIAMRoleID:    "https://console.aws.amazon.com/iam/home#/roles",
				tfSubnetID:     "",
				tfFunctionID:   functionURL,
				kubernetesID:   "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			outputResources := []any{}
			for id := range tt.want {
				or := map[string]any{"id": id}
				if arn, ok := arns[id]; ok {
					or["providerResourceId"] = arn
				}
				outputResources = append(outputResources, or)
			}
			graph := computeGraph([]generated.GenericResource{{
				ID:   to.Ptr(databaseID),
				Name: to.Ptr("db"),
				Type: to.Ptr("Radius.Data/mySqlDatabases"),
				Properties: map[string]any{
					"application": appID,
					"status":      map[string]any{"outputResources": outputResources},
				},
			}}, nil, tt.tenantID, nil)

			require.Len(t, graph.Resources, 1)

			// Check the wire format that HTTP clients see, not just the Go struct.
			raw, err := json.Marshal(graph)
			require.NoError(t, err)
			var decoded struct {
				Resources []struct {
					OutputResources []struct {
						ID        string `json:"id"`
						PortalURL string `json:"portalUrl"`
					} `json:"outputResources"`
				} `json:"resources"`
			}
			require.NoError(t, json.Unmarshal(raw, &decoded))
			require.Len(t, decoded.Resources, 1)

			got := map[string]string{}
			for _, outputResource := range decoded.Resources[0].OutputResources {
				got[outputResource.ID] = outputResource.PortalURL
			}
			require.Equal(t, tt.want, got)
		})
	}
}

func Test_containsAzureOutputResource(t *testing.T) {
	t.Parallel()

	const (
		azureID      = "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg/providers/Microsoft.Storage/storageAccounts/mystorage"
		awsID        = "/planes/aws/aws/accounts/123456789012/regions/us-west-2/providers/AWS.S3/Bucket/mybucket"
		kubernetesID = "/planes/kubernetes/local/namespaces/default/providers/apps/Deployment/foo"
	)

	withOutputResources := func(ids ...string) generated.GenericResource {
		items := make([]any, 0, len(ids))
		for _, id := range ids {
			items = append(items, map[string]any{"id": id})
		}
		return generated.GenericResource{
			Properties: map[string]any{"status": map[string]any{"outputResources": items}},
		}
	}

	tests := []struct {
		name string
		list []generated.GenericResource
		want bool
	}{
		{
			name: "no resources",
		},
		{
			name: "only non-Azure output resources",
			list: []generated.GenericResource{withOutputResources(kubernetesID, awsID)},
		},
		{
			name: "Azure output resource on a later resource",
			list: []generated.GenericResource{withOutputResources(kubernetesID), withOutputResources(awsID, azureID)},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, containsAzureOutputResource(tt.list))
		})
	}
}

func Test_computeGraphPayload_FiltersResourcesWhileListing(t *testing.T) {
	t.Parallel()

	const (
		applicationScope = "/planes/radius/local/resourcegroups/default"
		otherScope       = "/planes/radius/local/resourcegroups/other"
		pageSize         = 2
		azureStorageID   = "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg/providers/Microsoft.Storage/storageAccounts/mystorage"

		// widgets declares a default API version. gadgets does not, so its lowest version is used.
		summary = `{"name":"Applications.Test","resourceTypes":{` +
			`"widgets":{"apiVersions":{"2023-10-01-preview":{},"2024-01-01":{}},"defaultApiVersion":"2024-01-01"},` +
			`"gadgets":{"apiVersions":{"2023-10-01-preview":{},"2022-01-01":{}}}}}`
	)
	applicationID := resources.MustParse(applicationScope + "/providers/Radius.Core/applications/myapp")
	listRequest := func(scope string, resourceType string, apiVersion string, page int) string {
		return fmt.Sprintf("GET %s/providers/applications.test/%s?api-version=%s&page=%d", scope, resourceType, apiVersion, page)
	}

	tests := []struct {
		name          string
		environmentID string
		failRequest   string
		wantRequests  map[string]int
	}{
		{
			name:          "environment in the application scope with different case",
			environmentID: "/planes/radius/local/resourceGroups/Default/providers/Radius.Core/environments/myenv",
			wantRequests: map[string]int{
				"GET /planes/radius/local/providers":                      1,
				listRequest(applicationScope, "widgets", "2024-01-01", 1): 1,
				listRequest(applicationScope, "widgets", "2024-01-01", 2): 1,
				listRequest(applicationScope, "gadgets", "2022-01-01", 1): 1,
				listRequest(applicationScope, "gadgets", "2022-01-01", 2): 1,
			},
		},
		{
			name:          "environment in another scope",
			environmentID: "/planes/radius/local/resourceGroups/other/providers/Radius.Core/environments/myenv",
			wantRequests: map[string]int{
				"GET /planes/radius/local/providers":                      1,
				listRequest(applicationScope, "widgets", "2024-01-01", 1): 1,
				listRequest(applicationScope, "widgets", "2024-01-01", 2): 1,
				listRequest(applicationScope, "gadgets", "2022-01-01", 1): 1,
				listRequest(otherScope, "widgets", "2024-01-01", 1):       1,
				listRequest(otherScope, "gadgets", "2022-01-01", 1):       1,
				listRequest(otherScope, "gadgets", "2022-01-01", 2):       1,
			},
		},
		{
			name:          "later page fails",
			environmentID: "/planes/radius/local/resourceGroups/other/providers/Radius.Core/environments/myenv",
			failRequest:   listRequest(otherScope, "gadgets", "2022-01-01", 2),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			environmentScope := resources.MustParse(tt.environmentID).RootScope()
			frontendID := applicationScope + "/providers/Applications.Test/widgets/frontend"
			sharedID := applicationScope + "/providers/Applications.Test/widgets/shared"
			backendID := environmentScope + "/providers/Applications.Test/gadgets/backend"
			databaseID := environmentScope + "/providers/Applications.Test/gadgets/database"
			otherEnvironmentID := environmentScope + "/providers/Radius.Core/environments/otherenv"

			// Matching resources are on the last page of each list.
			fixtureResources := []generated.GenericResource{
				{
					ID: to.Ptr(applicationScope + "/providers/Applications.Test/widgets/unrelated"),
					Properties: map[string]any{
						"application": applicationScope + "/providers/Radius.Core/applications/otherapp",
						// An unrelated resource's Azure output resource must not trigger the tenant lookup.
						"status": map[string]any{"outputResources": []any{map[string]any{"id": azureStorageID}}},
					},
				},
				{
					ID:         to.Ptr(applicationScope + "/providers/Applications.Test/widgets/unassigned"),
					Properties: map[string]any{},
				},
				{
					ID: to.Ptr(frontendID),
					Properties: map[string]any{
						"application": applicationID.String(),
						"connections": map[string]any{"backend": map[string]any{"source": backendID}},
					},
				},
				{
					ID:         to.Ptr(sharedID),
					Properties: map[string]any{"application": applicationID.String(), "environment": tt.environmentID},
				},
				{
					ID:         to.Ptr(environmentScope + "/providers/Applications.Test/gadgets/unrelated1"),
					Properties: map[string]any{"environment": otherEnvironmentID},
				},
				{
					ID:         to.Ptr(environmentScope + "/providers/Applications.Test/gadgets/unrelated2"),
					Properties: map[string]any{"environment": otherEnvironmentID},
				},
				{
					// The graph reaches database only through the connections of backend, an
					// environment resource, so database is present only if backend was listed.
					ID: to.Ptr(backendID),
					Properties: map[string]any{
						"environment": tt.environmentID,
						"connections": map[string]any{"database": map[string]any{"source": databaseID}},
					},
				},
				{
					ID:         to.Ptr(databaseID),
					Properties: map[string]any{"environment": tt.environmentID},
				},
			}

			var mu sync.Mutex
			requests := map[string]int{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				path := strings.ToLower(r.URL.Path)
				request := r.Method + " " + path
				page := requestedPage(t, r)
				_, _, isList := strings.Cut(path, "/providers/applications.test/")
				if isList {
					request += fmt.Sprintf("?api-version=%s&page=%d", r.URL.Query().Get("api-version"), page)
				}
				mu.Lock()
				requests[request]++
				mu.Unlock()

				switch {
				case request == tt.failRequest:
					// Use a status that the client does not retry.
					w.WriteHeader(http.StatusBadRequest)
				case path == "/planes/radius/local/providers":
					writeTestResponse(t, w, json.RawMessage(`{"value":[`+summary+`]}`))
				case path == "/planes/radius/local/providers/applications.test":
					writeTestResponse(t, w, json.RawMessage(summary))
				case isList:
					items := []generated.GenericResource{}
					for _, resource := range fixtureResources {
						if strings.HasPrefix(strings.ToLower(*resource.ID), path+"/") {
							items = append(items, resource)
						}
					}
					writeTestListPage(t, w, r, items, page, pageSize)
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(server.Close)

			conn, err := sdk.NewDirectConnection(server.URL)
			require.NoError(t, err)

			payload, err := computeGraphPayload(t.Context(), applicationID, tt.environmentID, conn, nil)
			if tt.failRequest != "" {
				require.Error(t, err)
				require.Nil(t, payload)
				return
			}
			require.NoError(t, err)

			gotIDs := make([]string, 0, len(payload.Resources))
			for _, resource := range payload.Resources {
				gotIDs = append(gotIDs, *resource.ID)
			}
			require.ElementsMatch(t, []string{frontendID, sharedID, backendID, databaseID}, gotIDs)

			mu.Lock()
			defer mu.Unlock()
			require.Equal(t, tt.wantRequests, requests)
		})
	}
}

func Test_visitResourcesByType_VisitsEachPageBeforeRequestingTheNext(t *testing.T) {
	t.Parallel()

	const rootScope = "/planes/radius/local/resourcegroups/default"
	items := make([]generated.GenericResource, 5)
	for i := range items {
		name := fmt.Sprintf("r%d", i)
		items[i] = generated.GenericResource{
			ID:   to.Ptr(rootScope + "/providers/Applications.Test/widgets/" + name),
			Name: to.Ptr(name),
		}
	}

	tests := []struct {
		name       string
		failPage   int
		wantEvents []string
	}{
		{
			name:       "all pages succeed",
			wantEvents: []string{"request page 1", "visit r0", "visit r1", "request page 2", "visit r2", "visit r3", "request page 3", "visit r4"},
		},
		{
			name:       "later page fails",
			failPage:   3,
			wantEvents: []string{"request page 1", "visit r0", "visit r1", "request page 2", "visit r2", "visit r3", "request page 3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var mu sync.Mutex
			events := []string{}
			record := func(event string) {
				mu.Lock()
				defer mu.Unlock()
				events = append(events, event)
			}

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				page := requestedPage(t, r)
				record(fmt.Sprintf("request page %d", page))
				if page == tt.failPage {
					// Use a status that the client does not retry.
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				writeTestListPage(t, w, r, items, page, 2)
			}))
			t.Cleanup(server.Close)

			conn, err := sdk.NewDirectConnection(server.URL)
			require.NoError(t, err)

			err = visitResourcesByType(t.Context(), rootScope, "Applications.Test/widgets", "2024-01-01", sdk.NewClientOptions(conn), func(resource generated.GenericResource) {
				record("visit " + to.String(resource.Name))
			})
			if tt.failPage != 0 {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			mu.Lock()
			defer mu.Unlock()
			require.Equal(t, tt.wantEvents, events)
		})
	}
}

// requestedPage returns the 1-based page number that a test list request asks for.
func requestedPage(t *testing.T, r *http.Request) int {
	t.Helper()

	value := r.URL.Query().Get("page")
	if value == "" {
		return 1
	}
	page, err := strconv.Atoi(value)
	if err != nil || page < 1 {
		t.Errorf("invalid page %q", value)
		return 1
	}
	return page
}

// writeTestListPage writes the given 1-based page of items, with a next link while items remain.
func writeTestListPage(t *testing.T, w http.ResponseWriter, r *http.Request, items []generated.GenericResource, page int, pageSize int) {
	t.Helper()

	start := min((page-1)*pageSize, len(items))
	end := min(start+pageSize, len(items))
	response := map[string]any{"value": items[start:end]}
	if end < len(items) {
		response["nextLink"] = fmt.Sprintf("%s?page=%d", r.URL.Path, page+1)
	}
	writeTestResponse(t, w, response)
}

// writeTestResponse writes value as JSON. It runs in the test server's handler goroutine, so it
// reports failures without stopping the test.
func writeTestResponse(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Errorf("failed to write test response: %v", err)
	}
}
