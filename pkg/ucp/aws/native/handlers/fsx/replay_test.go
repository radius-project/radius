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

package fsx

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/fsx"

	"github.com/radius-project/radius/pkg/ucp/aws/native/awsreplay"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// Test_Replay_FileSystem creates the smallest OpenZFS file system, changes its maintenance window and tags, and
// deletes it, against AWS's recorded responses. Deleting it takes a final backup, tagged like the file system, which
// the recording deletes afterwards. AWS fills in defaults inside OpenZFSConfiguration, so the test checks the other
// values.
func Test_Replay_FileSystem(t *testing.T) {
	s := awsreplay.New(t, "testdata/filesystem.json")
	h := NewFileSystem(fsx.NewFromConfig(s.Config(t.Context())))
	run := s.Input("RADIUS_REC_RUN")
	zfs := func(window string) map[string]any {
		return map[string]any{
			"DeploymentType":               "SINGLE_AZ_1",
			"ThroughputCapacity":           64.0,
			"AutomaticBackupRetentionDays": 0.0,
			"WeeklyMaintenanceStartTime":   window,
		}
	}
	properties := map[string]any{
		"FileSystemType":       "OPENZFS",
		"StorageCapacity":      64.0,
		"StorageType":          "SSD",
		"SubnetIds":            []any{s.Input("RADIUS_REC_SUBNET")},
		"SecurityGroupIds":     []any{s.Input("RADIUS_REC_SECURITY_GROUP")},
		"OpenZFSConfiguration": zfs("1:05:00"),
		"Tags":                 tags("Name", "radius-rec-"+run+"-zfs", "radius-recording", run),
	}
	updated := with(properties, map[string]any{
		"OpenZFSConfiguration": zfs("2:05:00"),
		"Tags":                 tags("Name", "radius-rec-"+run+"-zfs", "radius-recording", run, "stage", "updated"),
	})
	nativetest.Lifecycle{
		Handler:     h,
		Session:     s,
		Properties:  properties,
		Want:        with(properties, map[string]any{"SecurityGroupIds": nil, "OpenZFSConfiguration": nil}),
		Update:      updated,
		WantUpdated: with(updated, map[string]any{"SecurityGroupIds": nil, "OpenZFSConfiguration": nil}),
		Poll:        30 * time.Second,
	}.Run(t)
}
