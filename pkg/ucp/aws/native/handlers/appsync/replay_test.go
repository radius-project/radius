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

package appsync

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/appsync"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/radius-project/radius/pkg/ucp/aws/native/awsreplay"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// Test_Replay_GraphQLSchema sets the schema of an existing GraphQL API from Definition, then from an S3 object,
// against AWS's recorded responses. AppSync can't remove a schema, so the test deletes nothing, and the recording
// deletes the API afterwards.
func Test_Replay_GraphQLSchema(t *testing.T) {
	s := awsreplay.New(t, "testdata/graphql-schema.json")
	cfg := s.Config(t.Context())
	api := s.Input("RADIUS_REC_APPSYNC_API_ID")
	properties := map[string]any{"ApiId": api, "Definition": "type Query { hello: String }"}
	nativetest.Lifecycle{
		Handler:     NewGraphQLSchema(appsync.NewFromConfig(cfg), s3.NewFromConfig(cfg)),
		Session:     s,
		Properties:  properties,
		Want:        map[string]any{"ApiId": api, "Id": api},
		Update:      map[string]any{"ApiId": api, "DefinitionS3Location": s.Input("RADIUS_REC_APPSYNC_SCHEMA_S3_LOCATION")},
		WantUpdated: map[string]any{"ApiId": api, "Id": api},
		Poll:        5 * time.Second,
		SkipDelete:  true,
	}.Run(t)
}
