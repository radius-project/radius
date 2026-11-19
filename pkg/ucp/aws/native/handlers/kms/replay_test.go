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

package kms

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/kms"

	"github.com/radius-project/radius/pkg/ucp/aws/native/awsreplay"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// Test_Replay_Grant creates a grant on a key the template names by key ID, reads it, and retires it, against AWS's
// responses. Every property is create-only, so there is no update step.
func Test_Replay_Grant(t *testing.T) {
	s := awsreplay.New(t, "testdata/grant.json")
	api := kms.NewFromConfig(s.Config(t.Context()))

	key := s.Input("RADIUS_REC_KMS_KEY_ID")
	properties := map[string]any{
		"KeyId":            key,
		"GranteePrincipal": s.Input("RADIUS_REC_GRANTEE_ROLE_ARN"),
		"Operations":       []any{"Decrypt", "GenerateDataKey"},
		"Constraints":      map[string]any{"EncryptionContextSubset": map[string]any{"app": "radius-rec"}},
	}
	account := s.Input("RADIUS_REC_ACCOUNT_ID")
	nativetest.Lifecycle{
		Handler:    NewGrant(api),
		Session:    s,
		Properties: properties,
		Want: with(properties, map[string]any{
			"KeyId":          "arn:aws:kms:" + awsreplay.Region + ":" + account + ":key/" + key,
			"IssuingAccount": "arn:aws:iam::" + account + ":root",
		}),
		Poll: 5 * time.Second,
	}.Run(t)
}
