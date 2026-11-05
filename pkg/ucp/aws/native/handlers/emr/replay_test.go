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

package emr

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/emr"

	"github.com/radius-project/radius/pkg/ucp/aws/native/awsreplay"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// Test_Replay_Cluster starts a one-node cluster, raises its step concurrency and changes its tags, and terminates it,
// against AWS's recorded responses.
func Test_Replay_Cluster(t *testing.T) {
	s := awsreplay.New(t, "testdata/cluster.json")
	h := NewCluster(emr.NewFromConfig(s.Config(t.Context())))
	run := s.Input("RADIUS_REC_RUN")
	properties := map[string]any{
		"Name":                 "radius-rec-" + run,
		"ReleaseLabel":         "emr-7.1.0",
		"ServiceRole":          s.Input("RADIUS_REC_EMR_SERVICE_ROLE"),
		"JobFlowRole":          s.Input("RADIUS_REC_EMR_INSTANCE_PROFILE"),
		"VisibleToAllUsers":    true,
		"StepConcurrencyLevel": 1.0,
		"Instances": map[string]any{
			"MasterInstanceGroup":         map[string]any{"InstanceCount": 1.0, "InstanceType": "m5.xlarge", "Market": "ON_DEMAND"},
			"Ec2SubnetId":                 s.Input("RADIUS_REC_SUBNET"),
			"KeepJobFlowAliveWhenNoSteps": true,
			"TerminationProtected":        false,
		},
		"Tags": tags("radius-recording", run),
	}
	// The update leaves out Instances: the lifecycle compares it with what AWS returns as it is, and AWS fills in
	// defaults there that EMR can't change.
	updated := with(properties, map[string]any{"Instances": nil, "StepConcurrencyLevel": 2.0, "Tags": tags("radius-recording", run, "stage", "updated")})
	nativetest.Lifecycle{
		Handler:     h,
		Session:     s,
		Properties:  properties,
		Want:        with(properties, map[string]any{"Instances": nil}),
		Update:      updated,
		WantUpdated: updated,
		Poll:        30 * time.Second,
	}.Run(t)
}
