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

package rds

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/rds/types"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
)

func Test_InRegion(t *testing.T) {
	opts := rds.Options{}
	inRegion("eu-west-2")(&opts)
	require.Equal(t, "eu-west-2", opts.Region)
}

func Test_ActivityStream_Identifier(t *testing.T) {
	h := NewActivityStream(newFakeAPI(awsfake.NewCloudControl()))
	id, err := h.Identifier(map[string]any{"ResourceArn": arnPrefix + "cluster:App-Aurora"})
	require.NoError(t, err)
	require.Equal(t, clusterArn, id, "RDS stores a cluster's ARN in lowercase")

	id, err = h.Identifier(map[string]any{})
	require.NoError(t, err)
	require.Empty(t, id)

	for _, value := range []string{arnPrefix + "db:app-oracle", "app-aurora", arnPrefix + "cluster:", "arn:aws:kms:us-west-2:123456789012:cluster:app"} {
		_, err := h.Identifier(map[string]any{"ResourceArn": value})
		var failure *native.Failure
		require.ErrorAs(t, err, &failure, value)
		require.True(t, failure.ClientError)
		require.Equal(t, native.OutcomeRejected, failure.Outcome)
	}
}

func Test_ActivityStream_Equivalent(t *testing.T) {
	tests := []struct {
		path     string
		template any
		aws      any
		want     bool
		recorded bool
	}{
		{"ResourceArn", arnPrefix + "cluster:APP-AURORA", clusterArn, true, false},
		{"ResourceArn", arnPrefix + "cluster:other", clusterArn, false, false},
		{"KmsKeyId", keyID, keyArn, true, false},
		{"KmsKeyId", keyArn, keyID, true, false},
		{"KmsKeyId", "other-key", keyArn, false, false},
		{"KmsKeyId", "alias/app-das", keyArn, false, true},
		{"KmsKeyId", "arn:aws:kms:us-west-2:123456789012:alias/app-das", keyID, false, true},
		{"Mode", "ASYNC", "async", false, false},
		{"KmsKeyId", 1, keyArn, false, false},
	}
	for _, tt := range tests {
		require.Equal(t, tt.want, streamEquivalent(tt.path, tt.template, tt.aws), "%s %v %v", tt.path, tt.template, tt.aws)
		require.Equal(t, tt.recorded, sameAlias(tt.path, tt.template, tt.aws), "%s %v %v", tt.path, tt.template, tt.aws)
	}
}

// startedStream returns a handler whose fake has the cluster app-aurora with a started stream.
func startedStream(t *testing.T) (*ActivityStream, *fakeAPI) {
	store := awsfake.NewCloudControl()
	addCluster(store, "app-aurora")
	api := newFakeAPI(store)
	h := NewActivityStream(api)
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: stream})
	require.NoError(t, err)
	progress, err := h.Progress(t.Context(), region, clusterArn, native.OperationCreate)
	require.NoError(t, err)
	require.Equal(t, native.Progress{State: "starting", Waiting: "started"}, progress)
	progress, err = h.Progress(t.Context(), region, clusterArn, native.OperationCreate)
	require.NoError(t, err)
	require.True(t, progress.Done)
	return h, api
}

func setCluster(api *fakeAPI, status string) {
	c := api.store.Resource(clusterType, "app-aurora")
	c["Status"] = status
	api.store.Put(clusterType, "app-aurora", c)
}

func Test_ActivityStream_Reset(t *testing.T) {
	t.Run("stops the stream and waits until it is stopped", func(t *testing.T) {
		h, _ := startedStream(t)
		require.NoError(t, h.Delete(t.Context(), region, clusterArn))
		progress, err := h.Progress(t.Context(), region, clusterArn, native.OperationDelete)
		require.NoError(t, err)
		require.Equal(t, native.Progress{State: "stopping", Waiting: "stopped"}, progress)
		_, err = h.Progress(t.Context(), region, clusterArn, native.OperationDelete)
		require.ErrorIs(t, err, native.ErrNotFound)
		require.ErrorIs(t, h.Reset(t.Context(), region, clusterArn), native.ErrNotFound, "a stopped stream is not found")
	})

	t.Run("a stream AWS is already stopping needs no call", func(t *testing.T) {
		h, api := startedStream(t)
		require.NoError(t, h.Reset(t.Context(), region, clusterArn))
		api.errs["StopActivityStream"] = fmt.Errorf("unexpected call")
		require.NoError(t, h.Reset(t.Context(), region, clusterArn))
	})

	t.Run("a cluster AWS is deleting stops its stream, so no call is needed", func(t *testing.T) {
		h, api := startedStream(t)
		setCluster(api, "deleting")
		api.errs["StopActivityStream"] = fmt.Errorf("unexpected call")
		require.NoError(t, h.Reset(t.Context(), region, clusterArn))
		progress, err := h.Progress(t.Context(), region, clusterArn, native.OperationDelete)
		require.NoError(t, err)
		require.False(t, progress.Done, "the delete waits until the cluster is gone")
		api.store.Remove(clusterType, "app-aurora")
		_, err = h.Progress(t.Context(), region, clusterArn, native.OperationDelete)
		require.ErrorIs(t, err, native.ErrNotFound)
	})

	t.Run("a cluster that can't stop the stream now fails the delete", func(t *testing.T) {
		h, api := startedStream(t)
		setCluster(api, "modifying")
		require.ErrorContains(t, h.Reset(t.Context(), region, clusterArn), "InvalidDBClusterStateFault")
	})

	t.Run("a stop AWS rejects for a stop that started since the read succeeds", func(t *testing.T) {
		h, api := startedStream(t)
		api.errs["StopActivityStream"] = fault("InvalidDBClusterStateFault")
		s := api.store.Resource(activityStreamType, clusterArn)
		s["Status"] = "stopping"
		describe := h.api
		h.api = stopRace{API: describe, before: func() { api.store.Put(activityStreamType, clusterArn, s) }}
		require.NoError(t, h.Reset(t.Context(), region, clusterArn))
	})

	t.Run("a cluster that is gone is not found", func(t *testing.T) {
		h, api := startedStream(t)
		api.store.Remove(clusterType, "app-aurora")
		require.ErrorIs(t, h.Reset(t.Context(), region, clusterArn), native.ErrNotFound)
		_, err := h.Read(t.Context(), region, clusterArn)
		require.ErrorIs(t, err, native.ErrNotFound)
	})
}

// stopRace calls before ahead of StopActivityStream, as when another stop starts between the handler's read and its
// call.
type stopRace struct {
	API
	before func()
}

func (s stopRace) StopActivityStream(ctx context.Context, in *rds.StopActivityStreamInput, opts ...func(*rds.Options)) (*rds.StopActivityStreamOutput, error) {
	s.before()
	return s.API.StopActivityStream(ctx, in, opts...)
}

func Test_ActivityStream_Progress(t *testing.T) {
	h, api := startedStream(t)
	stopped := api.store.Resource(activityStreamType, clusterArn)
	api.store.Remove(activityStreamType, clusterArn)
	_, err := h.Progress(t.Context(), region, clusterArn, native.OperationCreate)
	require.ErrorIs(t, err, native.ErrNotFound, "a stream that didn't start reads as absent, so the next deploy starts it")

	stopped["Status"] = "stopping"
	api.store.Put(activityStreamType, clusterArn, stopped)
	_, err = h.Progress(t.Context(), region, clusterArn, native.OperationUpdate)
	require.ErrorIs(t, err, native.ErrNotFound, "a stream something is stopping reads as absent")

	api.errs["DescribeDBClusters"] = fault("ThrottlingException")
	_, err = h.Progress(t.Context(), region, clusterArn, native.OperationCreate)
	require.ErrorContains(t, err, "ThrottlingException")
}

func Test_ActivityStream_CreateAndUpdate(t *testing.T) {
	store := awsfake.NewCloudControl()
	addCluster(store, "app-aurora")
	api := newFakeAPI(store)
	h := NewActivityStream(api)

	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(stream, map[string]any{"ResourceArn": arnPrefix + "db:app"})})
	require.Error(t, err, "a DB instance's ARN is rejected before any call")
	require.Zero(t, store.Count(activityStreamType))

	setCluster(api, "modifying")
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: stream})
	require.ErrorContains(t, err, "InvalidDBClusterStateFault", "AWS's error reaches the user")

	require.NoError(t, h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: clusterArn, Desired: stream}))
}

func Test_ActivityStream_List(t *testing.T) {
	h, api := startedStream(t)
	for i := range 120 {
		addCluster(api.store, fmt.Sprintf("other-%03d", i))
	}
	resources, err := h.List(t.Context(), region)
	require.NoError(t, err)
	require.Len(t, resources, 1, "only clusters with a started stream are listed")
	require.Equal(t, clusterArn, resources[0].Identifier)
	require.Equal(t, map[string]any{
		"ResourceArn":       clusterArn,
		"Mode":              "async",
		"KmsKeyId":          keyID,
		"KinesisStreamName": "aws-rds-das-cluster-" + resourceID("app-aurora"),
	}, resources[0].Properties)

	api.errs["DescribeDBClusters"] = fault("ThrottlingException")
	_, err = h.List(t.Context(), region)
	require.ErrorContains(t, err, "ThrottlingException")
}

// emptyDescribe returns no clusters, as for an identifier AWS doesn't match.
type emptyDescribe struct {
	API
}

func (emptyDescribe) DescribeDBClusters(context.Context, *rds.DescribeDBClustersInput, ...func(*rds.Options)) (*rds.DescribeDBClustersOutput, error) {
	return &rds.DescribeDBClustersOutput{}, nil
}

func Test_ActivityStream_ReadEdges(t *testing.T) {
	h, api := startedStream(t)
	require.Empty(t, streamProperties(&types.DBCluster{}), "fields AWS leaves empty are left out")

	h.api = emptyDescribe{API: api}
	_, err := h.Read(t.Context(), region, clusterArn)
	require.ErrorIs(t, err, native.ErrNotFound)

	h.api = stopRace{API: api, before: func() { api.store.Remove(clusterType, "app-aurora") }}
	api.errs["StopActivityStream"] = fault("InvalidDBClusterStateFault")
	require.ErrorIs(t, h.Reset(t.Context(), region, clusterArn), native.ErrNotFound, "a cluster deleted since the read is not found")
}
