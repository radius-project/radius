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

package transfer

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/transfer"

	"github.com/radius-project/radius/pkg/ucp/aws/native/awsreplay"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// Test_Replay_Access gives a directory group access to a Transfer Family server whose identity provider is AWS
// Directory Service, changes its home directory, and deletes it, against AWS's responses. DescribeAccess leaves out
// Role, as another AWS client reports, and the handler doesn't read it.
func Test_Replay_Access(t *testing.T) {
	s := awsreplay.New(t, "testdata/access.json")
	properties := map[string]any{
		"ServerId":      s.Input("RADIUS_REC_TRANSFER_SERVER"),
		"ExternalId":    s.Input("RADIUS_REC_TRANSFER_GROUP_SID"),
		"Role":          s.Input("RADIUS_REC_TRANSFER_ROLE_ARN"),
		"HomeDirectory": "/" + s.Input("RADIUS_REC_TRANSFER_BUCKET") + "/home",
	}
	nativetest.Lifecycle{
		Handler:     NewAccess(transfer.NewFromConfig(s.Config(t.Context()))),
		Session:     s,
		Properties:  properties,
		Want:        with(properties, map[string]any{"Role": nil, "HomeDirectoryType": "PATH"}),
		Update:      with(properties, map[string]any{"HomeDirectory": "/" + s.Input("RADIUS_REC_TRANSFER_BUCKET") + "/shared"}),
		WantUpdated: with(properties, map[string]any{"Role": nil, "HomeDirectory": "/" + s.Input("RADIUS_REC_TRANSFER_BUCKET") + "/shared", "HomeDirectoryType": "PATH"}),
		Poll:        2 * time.Second,
	}.Run(t)
}
