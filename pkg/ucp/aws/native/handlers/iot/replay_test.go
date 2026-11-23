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

package iot

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/iot"

	"github.com/radius-project/radius/pkg/ucp/aws/native/awsreplay"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// Test_Replay_ThingGroupMembership adds a thing to a static thing group, and removes it, against AWS's responses.
// Every property is create-only, so there is no update step.
func Test_Replay_ThingGroupMembership(t *testing.T) {
	s := awsreplay.New(t, "testdata/thinggroupmembership.json")
	properties := membership(s.Input("RADIUS_REC_IOT_THING_GROUP"), s.Input("RADIUS_REC_IOT_THING"))
	nativetest.Lifecycle{
		Handler:    NewThingGroupMembership(iot.NewFromConfig(s.Config(t.Context()))),
		Session:    s,
		Properties: properties,
		Poll:       2 * time.Second,
	}.Run(t)
}
