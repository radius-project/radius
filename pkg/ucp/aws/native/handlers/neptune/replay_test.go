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

package neptune

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/neptune"

	"github.com/radius-project/radius/pkg/ucp/aws/native/awsreplay"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// Test_Replay_ClusterEndpoint creates a custom endpoint of a Neptune cluster over one of its instances, changes its
// type, and deletes it, against AWS's responses.
func Test_Replay_ClusterEndpoint(t *testing.T) {
	s := awsreplay.New(t, "testdata/clusterendpoint.json")
	api := neptune.NewFromConfig(s.Config(t.Context()))
	run := s.Input("RADIUS_REC_RUN")

	properties := map[string]any{
		"DBClusterEndpointIdentifier": "radius-rec-" + run,
		"DBClusterIdentifier":         s.Input("RADIUS_REC_NEPTUNE_CLUSTER"),
		"EndpointType":                "READER",
		"StaticMembers":               []any{s.Input("RADIUS_REC_NEPTUNE_INSTANCE")},
		"Tags":                        tags("radius-recording", run),
	}
	nativetest.Lifecycle{
		Handler:     NewClusterEndpoint(api),
		Session:     s,
		Properties:  properties,
		Want:        with(properties, map[string]any{"Status": "available"}),
		Update:      with(properties, map[string]any{"EndpointType": "ANY"}),
		WantUpdated: with(properties, map[string]any{"EndpointType": "ANY", "Status": "available"}),
		Poll:        15 * time.Second,
	}.Run(t)
}
