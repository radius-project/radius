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

package amplify

import (
	"maps"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/amplify"

	"github.com/radius-project/radius/pkg/ucp/aws/native/awsreplay"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// Test_Replay_BackendEnvironment creates a backend environment in a Gen 1 app and deletes it, against AWS's
// responses. Every property is create-only, so there is no update step.
func Test_Replay_BackendEnvironment(t *testing.T) {
	s := awsreplay.New(t, "testdata/backendenvironment.json")
	api := amplify.NewFromConfig(s.Config(t.Context()))

	app := s.Input("RADIUS_REC_AMPLIFY_APP_ID")
	properties := map[string]any{"AppId": app, "EnvironmentName": "radiusrec", "StackName": "amplify-radiusrec"}
	want := maps.Clone(properties)
	want["BackendEnvironmentArn"] = "arn:aws:amplify:us-west-2:123456789012:apps/" + app + "/backendenvironments/radiusrec"
	nativetest.Lifecycle{
		Handler:    NewBackendEnvironment(api),
		Session:    s,
		Properties: properties,
		Want:       want,
		Poll:       5 * time.Second,
	}.Run(t)
}
