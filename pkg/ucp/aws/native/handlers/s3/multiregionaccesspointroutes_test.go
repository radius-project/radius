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

package s3

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
)

// routeItems returns the properties of the test Multi-Region Access Point with routes.
func routeItems(routes ...map[string]any) map[string]any {
	list := []any{}
	for _, r := range routes {
		list = append(list, r)
	}
	return map[string]any{"Mrap": mrapARN(testMrap), "Routes": list}
}

// byBucket returns a route that names its bucket only.
func byBucket(bucket string, dial int) map[string]any {
	return map[string]any{"Bucket": bucket, "TrafficDialPercentage": dial}
}

func Test_Routes_Apply(t *testing.T) {
	ctx := t.Context()
	update := func(h *MultiRegionAccessPointRoutes, properties map[string]any) error {
		return h.Update(ctx, native.UpdateRequest{Region: region, Identifier: mrapARN(testMrap), Desired: properties})
	}

	t.Run("routes named by bucket or Region, in one call", func(t *testing.T) {
		api := newFakeAPI(awsfake.NewCloudControl())
		h := NewMultiRegionAccessPointRoutes(api)
		properties := routeItems(byBucket("bucket-east", 0), map[string]any{"Region": "us-west-2", "TrafficDialPercentage": 100},
			map[string]any{"Bucket": "bucket-eu", "Region": "eu-west-1", "TrafficDialPercentage": 0})
		require.NoError(t, update(h, properties))
		require.Equal(t, []string{"GetMultiRegionAccessPointRoutes", "SubmitMultiRegionAccessPointRoutes"}, api.calls)
		props, err := h.Read(ctx, region, mrapARN(testMrap))
		require.NoError(t, err)
		require.Equal(t, routesProperties(testMrap, 0, 100, 0), props)
	})

	for _, tc := range []struct {
		name       string
		properties map[string]any
		message    string
	}{
		{"a route left out", routeItems(byBucket("bucket-east", 100), byBucket("bucket-west", 0)), "leaves out the routes to bucket bucket-eu"},
		{"no routes", map[string]any{"Mrap": mrapARN(testMrap)}, "leaves out the routes to bucket bucket-east, bucket bucket-west, bucket bucket-eu"},
		{"a bucket the Multi-Region Access Point doesn't have", routeItems(byBucket("bucket-east", 100), byBucket("bucket-west", 0), byBucket("bucket-eu", 0), byBucket("bucket-ap", 0)), "lists the bucket bucket-ap"},
		{"a bucket in another Region", routeItems(map[string]any{"Bucket": "bucket-east", "Region": "eu-west-1", "TrafficDialPercentage": 100}, byBucket("bucket-west", 0), byBucket("bucket-eu", 0)), "the bucket is in us-east-1"},
		{"one route named by its bucket and by its Region", routeItems(byBucket("bucket-east", 100), map[string]any{"Region": "us-east-1", "TrafficDialPercentage": 0}, byBucket("bucket-west", 0), byBucket("bucket-eu", 0)), "lists the route to the bucket bucket-east twice"},
		{"a Region the Multi-Region Access Point doesn't have", routeItems(byBucket("bucket-east", 100), byBucket("bucket-west", 0), byBucket("bucket-eu", 0), map[string]any{"Region": "ap-south-1", "TrafficDialPercentage": 0}), "lists the Region ap-south-1"},
		{"the same Region twice", routeItems(map[string]any{"Region": "us-east-1", "TrafficDialPercentage": 100}, map[string]any{"Region": "us-east-1", "TrafficDialPercentage": 0}), "lists Region us-east-1 twice"},
		{"a dial that isn't a whole number", routeItems(byBucket("bucket-east", 0), map[string]any{"Bucket": "bucket-west", "TrafficDialPercentage": 50.5}), "whole number"},
		{"a route without a dial", routeItems(map[string]any{"Bucket": "bucket-east"}), "TrafficDialPercentage from 0 to 100"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := newFakeAPI(awsfake.NewCloudControl())
			err := update(NewMultiRegionAccessPointRoutes(api), tc.properties)
			requireRejected(t, err, tc.message)
			require.NotContains(t, api.calls, "SubmitMultiRegionAccessPointRoutes", "nothing changes in AWS")
		})
	}

	t.Run("a Multi-Region Access Point AWS doesn't have", func(t *testing.T) {
		h := NewMultiRegionAccessPointRoutes(newFakeAPI(awsfake.NewCloudControl()))
		missing := mrapARN(missingPrefix + ".mrap")
		_, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: map[string]any{"Mrap": missing, "Routes": routeItems(byBucket("bucket-east", 100))["Routes"]}})
		require.ErrorContains(t, err, noSuchMrapCode)
		_, err = h.Read(ctx, region, missing)
		require.ErrorIs(t, err, native.ErrNotFound)
		require.ErrorIs(t, h.Reset(ctx, region, missing), native.ErrNotFound)
	})

	t.Run("AWS rejects a configuration with every route passive", func(t *testing.T) {
		err := update(NewMultiRegionAccessPointRoutes(newFakeAPI(awsfake.NewCloudControl())), routesProperties(testMrap, 0, 0, 0))
		require.ErrorContains(t, err, invalidRequestCode)
	})
}

func Test_Routes_Reset(t *testing.T) {
	ctx := t.Context()
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewMultiRegionAccessPointRoutes(api)

	require.NoError(t, h.Reset(ctx, region, mrapARN(testMrap)))
	require.Equal(t, []string{"GetMultiRegionAccessPointRoutes"}, api.calls, "every route is already active, so nothing is sent")

	require.NoError(t, h.Update(ctx, native.UpdateRequest{Region: region, Identifier: mrapARN(testMrap), Desired: routesProperties(testMrap, 100, 0, 0)}))
	api.calls = nil
	require.NoError(t, h.Delete(ctx, region, mrapARN(testMrap)))
	require.Equal(t, []string{"GetMultiRegionAccessPointRoutes", "SubmitMultiRegionAccessPointRoutes"}, api.calls)
	props, err := h.Read(ctx, region, mrapARN(testMrap))
	require.NoError(t, err)
	require.Equal(t, routesProperties(testMrap, 100, 100, 100), props)

	t.Run("a Multi-Region Access Point with one bucket is already active", func(t *testing.T) {
		require.NoError(t, h.Reset(ctx, region, mrapARN(singlePrefix+".mrap")))
	})
}

func Test_Routes_Identifier(t *testing.T) {
	h := NewMultiRegionAccessPointRoutes(nil)
	id, err := h.Identifier(map[string]any{"Mrap": mrapARN(testMrap)})
	require.NoError(t, err, "existing gives only the Multi-Region Access Point")
	require.Equal(t, mrapARN(testMrap), id)

	id, err = h.Identifier(map[string]any{})
	require.NoError(t, err)
	require.Empty(t, id)

	_, err = h.Identifier(map[string]any{"Mrap": testMrap})
	requireRejected(t, err, "Mrap must be the ARN")
	_, err = h.Identifier(map[string]any{"Mrap": mrapARN(testMrap) + "/routes"})
	requireRejected(t, err, "Mrap must be the ARN")
	_, err = h.Identifier(map[string]any{"Mrap": mrapARN(strings.Repeat("a", 200))})
	requireRejected(t, err, "Mrap must be the ARN")
	_, err = h.Identifier(map[string]any{"Mrap": mrapARN(testMrap), "Routes": "all"})
	requireRejected(t, err, "each item of Routes")
}

func Test_Routes_IdentifierOfAnotherForm(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewMultiRegionAccessPointRoutes(api)
	_, err := h.Read(t.Context(), region, testMrap)
	requireRejected(t, err, "Mrap must be the ARN")
	requireRejected(t, h.Reset(t.Context(), region, testMrap), "Mrap must be the ARN")
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: map[string]any{"Mrap": testMrap, "Routes": routeItems(byBucket("bucket-east", 100))["Routes"]}})
	requireRejected(t, err, "Mrap must be the ARN")
	require.Empty(t, api.calls, "nothing reaches AWS")
}
