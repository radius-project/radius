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

	"github.com/radius-project/radius/pkg/ucp/aws/records"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
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

// Test_Tombstone_RepeatedDelete is the repro from review: the alias app-bucket manages the bucket assets, and an
// unrelated bucket is named app-bucket. Once the first DELETE removes assets, repeating the same DELETE returns 204 and
// never falls back to deleting app-bucket by the URL.
func Test_Tombstone_RepeatedDelete(t *testing.T) {
	h := newHarness(t)
	h.cc.Put(bucketType, "app-bucket", map[string]any{"BucketName": "app-bucket"})
	require.Equal(t, http.StatusCreated, h.put(bktPath, map[string]any{"BucketName": "assets"}).status)
	h.finish()

	require.Equal(t, http.StatusAccepted, h.delete(bktPath, testOwner).status)
	h.finish()
	require.Nil(t, h.cc.Resource(bucketType, "assets"))
	h.requireTombstone(bktPath)
	h.cc.Calls()

	for _, owner := range []string{testOwner, otherOwner, ""} {
		resp := h.delete(bktPath, owner)
		require.Equal(t, http.StatusNoContent, resp.status, resp.raw)
	}
	require.Empty(t, h.cc.Calls())
	require.NotNil(t, h.cc.Resource(bucketType, "app-bucket"), "the unrelated bucket is left alone")
}

// Test_Tombstone_RemovedRecord checks that every path that removes a record, not only a finished delete, leaves a
// tombstone, so a repeated DELETE does not fall back to the URL either.
func Test_Tombstone_RemovedRecord(t *testing.T) {
	h := newHarness(t)
	h.cc.Put(bucketType, "app-bucket", map[string]any{"BucketName": "app-bucket"})
	h.saveRecord(bktPath, func(rec *datamodel.AWSResource) { rec.Properties.AWSIdentifier = "gone" })

	// AWS no longer has the record's bucket.
	require.Equal(t, http.StatusNoContent, h.delete(bktPath, testOwner).status)
	h.requireTombstone(bktPath)
	h.cc.Calls()
	require.Equal(t, http.StatusNoContent, h.delete(bktPath, testOwner).status)
	require.Empty(t, h.cc.Calls())
	require.NotNil(t, h.cc.Resource(bucketType, "app-bucket"))
}

// Test_Tombstone_GetAndPut checks that a GET of a tombstone returns 404 without the GET fallback, and a PUT starts the
// alias over in the same record.
func Test_Tombstone_GetAndPut(t *testing.T) {
	h := newHarness(t)
	h.cc.Put(bucketType, "app-bucket", map[string]any{"BucketName": "app-bucket"})
	require.Equal(t, http.StatusCreated, h.put(bktPath, map[string]any{"BucketName": "assets"}).status)
	h.finish()
	require.Equal(t, http.StatusAccepted, h.delete(bktPath, testOwner).status)
	h.finish()
	h.cc.Calls()

	require.Equal(t, http.StatusNotFound, h.get(bktPath).status)
	require.Empty(t, h.cc.Calls(), "a tombstone is not read from AWS by the URL")

	// Another owner can use the alias again, and the record is reused.
	resp := h.putBody(bktPath, otherOwner, map[string]any{"properties": map[string]any{"BucketName": "assets"}})
	require.Equal(t, http.StatusCreated, resp.status, resp.raw)
	h.finish()
	rec := h.record(bktPath)
	require.False(t, records.IsDeleted(rec))
	require.Equal(t, otherOwner, rec.Properties.Owner)
	require.Equal(t, "assets", rec.Properties.AWSIdentifier)
	require.NotNil(t, h.cc.Resource(bucketType, "assets"))

	get := h.get(bktPath)
	require.Equal(t, http.StatusOK, get.status, get.raw)
	require.Equal(t, "assets", get.body["properties"].(map[string]any)["BucketName"])
}

// Test_Tombstone_NeverHoldsAnIdentifier checks that a tombstone keeps no AWS identifier, client token, or pending
// values, so another alias can create, or take over, the resource the deleted alias managed without a 409.
func Test_Tombstone_NeverHoldsAnIdentifier(t *testing.T) {
	h := newHarness(t)
	require.Equal(t, http.StatusCreated, h.put(bktPath, map[string]any{"BucketName": "assets"}).status)
	h.finish()
	require.Equal(t, http.StatusAccepted, h.delete(bktPath, testOwner).status)
	h.finish()

	rec := h.record(bktPath)
	require.True(t, records.IsDeleted(rec))
	require.Empty(t, rec.Properties.AWSIdentifier)
	require.Empty(t, rec.Properties.ClientToken)
	require.Nil(t, rec.Properties.PendingLastApplied)
	require.Empty(t, rec.Properties.PendingOperation)
	require.Nil(t, rec.Properties.LastApplied)

	// Another alias creates a bucket with the same name.
	resp := h.put(bkt2Path, map[string]any{"BucketName": "assets"})
	require.Equal(t, http.StatusCreated, resp.status, resp.raw)
	h.finish()
	require.Equal(t, "assets", h.record(bkt2Path).Properties.AWSIdentifier)
	require.True(t, records.IsDeleted(h.record(bktPath)))
}

// Test_Tombstone_TakeOver checks that a tombstone does not block another alias taking over the resource by name.
func Test_Tombstone_TakeOver(t *testing.T) {
	h := newHarness(t)
	h.saveRecord(bktPath, func(rec *datamodel.AWSResource) { rec.Properties.AWSIdentifier = "gone" })
	require.Equal(t, http.StatusNoContent, h.delete(bktPath, testOwner).status)
	h.requireTombstone(bktPath)

	h.cc.Put(bucketType, "gone", map[string]any{"BucketName": "gone"})
	resp := h.putBody(bkt2Path, testOwner, map[string]any{"name": "gone", "properties": map[string]any{"BucketName": "gone"}})
	require.Contains(t, []int{http.StatusOK, http.StatusCreated}, resp.status, resp.raw)
	require.Equal(t, "gone", h.record(bkt2Path).Properties.AWSIdentifier)
}
