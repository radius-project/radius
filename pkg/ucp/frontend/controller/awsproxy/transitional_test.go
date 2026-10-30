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

package awsproxy

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// TRANSITIONAL: these tests cover the guard on POST /:put, POST /:delete, and a DELETE that finds no record. Remove
// them with those paths.

const bucketCollection = providers + "AWS.S3/Bucket"

// transitionalPaths sends each transitional request for the bucket name.
var transitionalPaths = []struct {
	name string
	send func(h *harness, bucket string) response
}{
	{name: "POST /:put", send: func(h *harness, bucket string) response {
		return h.do(NewCreateOrUpdateAWSResourceWithPost, http.MethodPost, bucketCollection+"/:put", "", map[string]any{"properties": map[string]any{"BucketName": bucket}})
	}},
	{name: "POST /:delete", send: func(h *harness, bucket string) response {
		return h.do(NewDeleteAWSResourceWithPost, http.MethodPost, bucketCollection+"/:delete", "", map[string]any{"properties": map[string]any{"BucketName": bucket}})
	}},
	{name: "DELETE with no record and no owner", send: func(h *harness, bucket string) response {
		return h.delete(bucketCollection+"/"+bucket, "")
	}},
	{name: "DELETE with no record and an owner", send: func(h *harness, bucket string) response {
		return h.delete(bucketCollection+"/"+bucket, otherOwner)
	}},
}

func Test_TransitionalPaths_ManagedResource(t *testing.T) {
	for _, tt := range transitionalPaths {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			require.Equal(t, http.StatusCreated, h.put(bktPath, map[string]any{"BucketName": "assets"}).status)
			h.finish()
			h.cc.Calls()

			resp := tt.send(h, "assets")
			require.Equal(t, http.StatusConflict, resp.status, resp.raw)
			require.Contains(t, resp.message(), `"app-bucket"`)
			require.Empty(t, h.cc.Calls(), "nothing is called in AWS")
			require.Equal(t, 1, h.cc.Count(bucketType))
			require.Equal(t, "assets", h.record(bktPath).Properties.AWSIdentifier)
		})
	}
}

func Test_TransitionalPaths_UnmanagedResource(t *testing.T) {
	for _, tt := range transitionalPaths {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			h.cc.Put(bucketType, "legacy", map[string]any{"BucketName": "legacy"})
			// A record that manages a different bucket does not block the request.
			require.Equal(t, http.StatusCreated, h.put(bktPath, map[string]any{"BucketName": "assets"}).status)
			h.finish()
			h.cc.Calls()

			resp := tt.send(h, "legacy")
			require.Contains(t, []int{http.StatusOK, http.StatusCreated, http.StatusAccepted}, resp.status, resp.raw)
			require.NotEmpty(t, h.cc.Calls())
		})
	}
}

func Test_TransitionalPaths_SearchFails(t *testing.T) {
	for _, tt := range transitionalPaths {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			h.db = brokenDB{Client: h.db, query: true}
			resp := tt.send(h, "legacy")
			require.Equal(t, http.StatusInternalServerError, resp.status, resp.raw)
			require.Empty(t, h.cc.Calls())
		})
	}
}
