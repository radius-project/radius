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

package iam

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/iam"

	"github.com/radius-project/radius/pkg/ucp/aws/native/awsreplay"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// Test_Replay_UserToGroupAddition adds two users to a group, replaces one of them, and removes the users it added,
// against AWS's recorded responses.
func Test_Replay_UserToGroupAddition(t *testing.T) {
	s := awsreplay.New(t, "testdata/usertogroupaddition.json")
	api := iam.NewFromConfig(s.Config(t.Context()))
	name := s.Input("RADIUS_REC_IAM_GROUP")
	one, two, three := s.Input("RADIUS_REC_IAM_USER_1"), s.Input("RADIUS_REC_IAM_USER_2"), s.Input("RADIUS_REC_IAM_USER_3")

	nativetest.Lifecycle{
		Handler:    NewUserToGroupAddition(api),
		Session:    s,
		Properties: group(name, one, two),
		Update:     group(name, two, three),
		Poll:       time.Second,
	}.Run(t)
}
