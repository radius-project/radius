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

package lambda

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/lambdamicrovms"

	"github.com/radius-project/radius/pkg/ucp/aws/native/awsreplay"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// Test_Replay_Microvm runs a MicroVM from an image the account has, with a short maximum duration, and terminates it,
// against AWS's responses. Every property is create-only, so there is no update step.
func Test_Replay_Microvm(t *testing.T) {
	s := awsreplay.New(t, "testdata/microvm.json")
	api := lambdamicrovms.NewFromConfig(s.Config(t.Context()))

	image := s.Input("RADIUS_REC_MICROVM_IMAGE_ARN")
	properties := map[string]any{
		"ImageIdentifier":          image,
		"IdlePolicy":               map[string]any{"AutoResumeEnabled": false, "MaxIdleDurationSeconds": 60, "SuspendedDurationSeconds": 0},
		"MaximumDurationInSeconds": 900,
		"Logging":                  map[string]any{"Disabled": map[string]any{}},
	}
	nativetest.Lifecycle{
		Handler:    NewMicrovm(api),
		Session:    s,
		Properties: properties,
		Want: map[string]any{
			"ImageIdentifier":          image,
			"ImageArn":                 image,
			"IdlePolicy":               properties["IdlePolicy"],
			"MaximumDurationInSeconds": 900,
			"State":                    "RUNNING",
		},
		Poll: 5 * time.Second,
	}.Run(t)
}
