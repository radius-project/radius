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
	"net/http"
	"net/http/httptest"
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
		kubernetesID   = "/planes/kubernetes/local/namespaces/default/providers/apps/Deployment/mydeployment"
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
				awsGovQueueID:  "https://console.amazonaws-us-gov.com/sqs/v3/home?region=us-gov-west-1#/queues",
				awsUnknownID:   "",
				tfBucketID:     "https://console.aws.amazon.com/s3/buckets",
				awsChinaRDSID:  "https://console.amazonaws.cn/rds/home?region=cn-north-1#databases:",
				tfIAMRoleID:    "https://console.aws.amazon.com/iam/home#/roles",
				tfSubnetID:     "",
				kubernetesID:   "",
			},
		},
		{
			name: "without an Azure tenant",
			want: map[string]string{
				azureStorageID: "",
				awsDatabaseID:  "https://us-west-2.console.aws.amazon.com/rds/home?region=us-west-2#databases:",
				awsGovQueueID:  "https://console.amazonaws-us-gov.com/sqs/v3/home?region=us-gov-west-1#/queues",
				awsUnknownID:   "",
				tfBucketID:     "https://console.aws.amazon.com/s3/buckets",
				awsChinaRDSID:  "https://console.amazonaws.cn/rds/home?region=cn-north-1#databases:",
				tfIAMRoleID:    "https://console.aws.amazon.com/iam/home#/roles",
				tfSubnetID:     "",
				kubernetesID:   "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			outputResources := []any{}
			for id := range tt.want {
				outputResources = append(outputResources, map[string]any{"id": id})
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

func Test_computeGraphPayload_ListsEachScopeOnce(t *testing.T) {
	t.Parallel()

	const summary = `{"name":"Applications.Test","resourceTypes":{"widgets":{"apiVersions":{"2023-10-01-preview":{}},"defaultApiVersion":"2023-10-01-preview"}}}`
	const applicationScope = "/planes/radius/local/resourcegroups/default"
	applicationID := resources.MustParse(applicationScope + "/providers/Radius.Core/applications/myapp")

	tests := []struct {
		name          string
		environmentID string
		wantRequests  map[string]int
	}{
		{
			name:          "environment in the application scope with different case",
			environmentID: "/planes/radius/local/resourceGroups/default/providers/Radius.Core/environments/myenv",
			wantRequests: map[string]int{
				"GET /planes/radius/local/providers":                                                  1,
				"GET /planes/radius/local/resourcegroups/default/providers/applications.test/widgets": 1,
			},
		},
		{
			name:          "environment in another scope",
			environmentID: "/planes/radius/local/resourceGroups/other/providers/Radius.Core/environments/myenv",
			wantRequests: map[string]int{
				"GET /planes/radius/local/providers":                                                  1,
				"GET /planes/radius/local/resourcegroups/default/providers/applications.test/widgets": 1,
				"GET /planes/radius/local/resourcegroups/other/providers/applications.test/widgets":   1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			frontendID := applicationScope + "/providers/Applications.Test/widgets/frontend"
			backendID := resources.MustParse(tt.environmentID).RootScope() + "/providers/Applications.Test/widgets/backend"
			widgets := []generated.GenericResource{
				{
					ID:   new(frontendID),
					Type: new("Applications.Test/widgets"),
					Properties: map[string]any{
						"application": applicationID.String(),
						"connections": map[string]any{"backend": map[string]any{"source": backendID}},
					},
				},
				{
					ID:         new(backendID),
					Type:       new("Applications.Test/widgets"),
					Properties: map[string]any{"environment": tt.environmentID},
				},
				{
					ID:         new(applicationScope + "/providers/Applications.Test/widgets/other"),
					Type:       new("Applications.Test/widgets"),
					Properties: map[string]any{"application": applicationScope + "/providers/Radius.Core/applications/otherapp"},
				},
			}

			var mu sync.Mutex
			requests := map[string]int{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				path := strings.ToLower(r.URL.Path)
				mu.Lock()
				requests[r.Method+" "+path]++
				mu.Unlock()

				w.Header().Set("Content-Type", "application/json")
				switch {
				case path == "/planes/radius/local/providers":
					_, _ = w.Write([]byte(`{"value":[` + summary + `]}`))
				case path == "/planes/radius/local/providers/applications.test":
					_, _ = w.Write([]byte(summary))
				case strings.HasSuffix(path, "/providers/applications.test/widgets"):
					scope := strings.TrimSuffix(path, "/providers/applications.test/widgets")
					inScope := []generated.GenericResource{}
					for _, widget := range widgets {
						if strings.HasPrefix(strings.ToLower(*widget.ID), scope+"/") {
							inScope = append(inScope, widget)
						}
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"value": inScope})
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			conn, err := sdk.NewDirectConnection(server.URL)
			require.NoError(t, err)

			payload, err := computeGraphPayload(t.Context(), applicationID, tt.environmentID, conn, nil)
			require.NoError(t, err)

			gotIDs := []string{}
			for _, resource := range payload.Resources {
				gotIDs = append(gotIDs, *resource.ID)
			}
			require.ElementsMatch(t, []string{frontendID, backendID}, gotIDs)

			mu.Lock()
			defer mu.Unlock()
			require.Equal(t, tt.wantRequests, requests)
		})
	}
}
