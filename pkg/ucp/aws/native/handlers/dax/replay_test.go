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

package dax

import (
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dax"

	"github.com/radius-project/radius/pkg/ucp/aws/native/awsreplay"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// Test_Replay_Cluster creates a one-node cluster, updates its description, and deletes it, against AWS's recorded
// responses. A cluster name has at most 20 characters, so RADIUS_REC_RUN has at most 9.
func Test_Replay_Cluster(t *testing.T) {
	s := awsreplay.New(t, "testdata/cluster.json")
	api := dax.NewFromConfig(s.Config(t.Context()))
	run := s.Input("RADIUS_REC_RUN")

	properties := map[string]any{
		"ClusterName":       "radius-rec-" + run,
		"NodeType":          "dax.t3.small",
		"ReplicationFactor": 1,
		"IAMRoleARN":        s.Input("RADIUS_REC_DAX_ROLE_ARN"),
		"SubnetGroupName":   s.Input("RADIUS_REC_DAX_SUBNET_GROUP"),
		"SecurityGroupIds":  []any{s.Input("RADIUS_REC_SECURITY_GROUP")},
		"Tags":              map[string]any{"radius-recording": run},
	}
	stored := map[string]any{
		"ClusterName":     strings.ToLower("radius-rec-" + run),
		"SubnetGroupName": strings.ToLower(s.Input("RADIUS_REC_DAX_SUBNET_GROUP")),
	}
	updated := with(properties, map[string]any{"Description": "radius recording"})
	nativetest.Lifecycle{
		Handler:     NewCluster(api),
		Session:     s,
		Properties:  properties,
		Want:        with(properties, stored),
		Update:      updated,
		WantUpdated: with(updated, stored),
		Poll:        30 * time.Second,
	}.Run(t)
}
