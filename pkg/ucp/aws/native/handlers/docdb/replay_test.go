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

package docdb

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/docdb"

	"github.com/radius-project/radius/pkg/ucp/aws/native/awsreplay"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// Test_Replay_ClusterAndInstance creates a cluster and an instance in it, updates both, and deletes them, against
// AWS's recorded responses. Deleting the cluster takes a final snapshot, which the recording deletes afterwards.
func Test_Replay_ClusterAndInstance(t *testing.T) {
	s := awsreplay.New(t, "testdata/cluster-and-instance.json")
	api := docdb.NewFromConfig(s.Config(t.Context()))
	run := s.Input("RADIUS_REC_RUN")
	password := "Rec-" + run + "-Pw1"
	s.Scrub(password)
	tags := tags("radius-recording", run)
	name := "radius-rec-" + run

	cluster := NewCluster(api)
	cluster.now = func() time.Time { return time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC) }
	properties := map[string]any{
		"DBClusterIdentifier":   name,
		"DBSubnetGroupName":     s.Input("RADIUS_REC_DOCDB_SUBNET_GROUP"),
		"VpcSecurityGroupIds":   []any{s.Input("RADIUS_REC_SECURITY_GROUP")},
		"MasterUsername":        "radius",
		"MasterUserPassword":    password,
		"EngineVersion":         "5.0",
		"BackupRetentionPeriod": 1,
		"Tags":                  tags,
	}
	clusterRun := nativetest.Lifecycle{
		Handler:    cluster,
		Session:    s,
		Properties: properties,
		Want:       with(properties, map[string]any{"MasterUserPassword": nil, "EngineVersion": "5.0.0"}),
		Poll:       time.Minute,
		Update:     with(properties, map[string]any{"BackupRetentionPeriod": 2, "MasterUserPassword": nil, "EngineVersion": nil}),
		WantUpdated: with(properties, map[string]any{
			"BackupRetentionPeriod": 2, "MasterUserPassword": nil, "EngineVersion": "5.0.0",
		}),
	}

	instance := NewInstance(api)
	instanceProperties := map[string]any{
		"DBInstanceIdentifier": name + "-1",
		"DBClusterIdentifier":  name,
		"DBInstanceClass":      "db.t3.medium",
		"Tags":                 tags,
	}
	instanceRun := nativetest.Lifecycle{
		Handler:     instance,
		Session:     s,
		Properties:  instanceProperties,
		Update:      with(instanceProperties, map[string]any{"PreferredMaintenanceWindow": "sun:10:00-sun:10:30"}),
		WantUpdated: with(instanceProperties, map[string]any{"PreferredMaintenanceWindow": "sun:10:00-sun:10:30"}),
		Poll:        time.Minute,
	}

	// The instance needs the cluster, so the cluster's lifecycle runs around it: create the cluster, then the
	// instance's whole lifecycle, then update and delete the cluster.
	clusterRun.Between = func() { instanceRun.Run(t) }
	clusterRun.Run(t)
}
