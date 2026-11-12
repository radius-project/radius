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

package dms

import (
	"strings"
	"testing"
	"time"

	dms "github.com/aws/aws-sdk-go-v2/service/databasemigrationservice"

	"github.com/radius-project/radius/pkg/ucp/aws/native/awsreplay"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// Test_Replay_ReplicationInstance creates the smallest private replication instance, changes its maintenance window,
// and deletes it, against AWS's recorded responses.
func Test_Replay_ReplicationInstance(t *testing.T) {
	s := awsreplay.New(t, "testdata/replication-instance.json")
	api := dms.NewFromConfig(s.Config(t.Context()))
	run := s.Input("RADIUS_REC_RUN")
	subnetGroup := s.Input("RADIUS_REC_DMS_SUBNET_GROUP")

	properties := map[string]any{
		"ReplicationInstanceIdentifier":    "radius-rec-" + run,
		"ReplicationInstanceClass":         "dms.t3.small",
		"AllocatedStorage":                 20,
		"MultiAZ":                          false,
		"PubliclyAccessible":               false,
		"ReplicationSubnetGroupIdentifier": subnetGroup,
		"VpcSecurityGroupIds":              []any{s.Input("RADIUS_REC_SECURITY_GROUP")},
		"Tags":                             tags("radius-recording", run),
	}
	stored := map[string]any{
		"Id":                               strings.ToLower("radius-rec-" + run),
		"ReplicationInstanceIdentifier":    strings.ToLower("radius-rec-" + run),
		"ReplicationSubnetGroupIdentifier": strings.ToLower(subnetGroup),
	}
	updated := with(properties, map[string]any{"PreferredMaintenanceWindow": "sun:06:00-sun:06:30"})
	nativetest.Lifecycle{
		Handler:     NewReplicationInstance(api),
		Session:     s,
		Properties:  properties,
		Want:        with(properties, stored),
		Update:      updated,
		WantUpdated: with(updated, stored),
		Poll:        30 * time.Second,
	}.Run(t)
}
