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

package redshift

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/redshift"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsreplay"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// The recordings need RADIUS_REC_RUN, a few random characters that keep the names of one recording apart from
// another's.

// Test_Replay_AuthenticationProfile creates a profile, replaces its content, and deletes it, against AWS's responses.
// AWS returns the content in its own form, with numbers as strings, without null values and spaces.
func Test_Replay_AuthenticationProfile(t *testing.T) {
	s := awsreplay.New(t, "testdata/authenticationprofile.json")
	api := redshift.NewFromConfig(s.Config(t.Context()))
	name := "radius-rec-" + s.Input("RADIUS_REC_RUN")
	nativetest.Lifecycle{
		Handler:     NewAuthenticationProfile(api),
		Session:     s,
		Properties:  profileProps(name, `{"AllowDBUserOverride": 1, "Unused": null}`),
		Want:        profileProps(name, `{"AllowDBUserOverride":"1"}`),
		Update:      profileProps(name, `{"App_ID": "other"}`),
		WantUpdated: profileProps(name, `{"App_ID":"other"}`),
		Poll:        5 * time.Second,
	}.Run(t)
}

// Test_Replay_AuthenticationProfileRejected creates a profile that holds a secret. AWS answers 400 with
// InvalidAuthenticationProfileRequestFault, which the handler reports as a rejected client failure, so UCP answers
// 400 and drops the record.
func Test_Replay_AuthenticationProfileRejected(t *testing.T) {
	s := awsreplay.New(t, "testdata/authenticationprofilerejected.json")
	api := redshift.NewFromConfig(s.Config(t.Context()))
	_, err := NewAuthenticationProfile(api).Create(t.Context(), native.CreateRequest{
		Region:     awsreplay.Region,
		Properties: profileProps("radius-rec-"+s.Input("RADIUS_REC_RUN"), `{"App_ID": "example", "Password": "not-a-secret"}`),
	})
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.ClientError)
	require.False(t, failure.Conflict)
	require.Equal(t, native.OutcomeRejected, failure.Outcome)
	require.Contains(t, failure.Message, profileInvalid)
}
