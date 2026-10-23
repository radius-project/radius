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

package aws

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeCloudFormationClient struct {
	calls atomic.Int32
	err   error
}

func (f *fakeCloudFormationClient) DescribeType(ctx context.Context, params *cloudformation.DescribeTypeInput, optFns ...func(*cloudformation.Options)) (*cloudformation.DescribeTypeOutput, error) {
	f.calls.Add(1)
	if f.err != nil {
		return nil, f.err
	}

	opts := cloudformation.Options{}
	for _, fn := range optFns {
		fn(&opts)
	}

	return &cloudformation.DescribeTypeOutput{
		TypeName: params.TypeName,
		Schema:   aws.String(opts.Region + "/" + aws.ToString(params.TypeName)),
	}, nil
}

func withRegion(region string) func(*cloudformation.Options) {
	return func(o *cloudformation.Options) {
		o.Region = region
	}
}

func describeTypeInput(typeName string) *cloudformation.DescribeTypeInput {
	return &cloudformation.DescribeTypeInput{
		Type:     types.RegistryTypeResource,
		TypeName: aws.String(typeName),
	}
}

func newTestDescribeTypeCache(client AWSCloudFormationClient, ttl time.Duration) (*DescribeTypeCache, *time.Time) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cache := NewDescribeTypeCache(client, ttl)
	cache.now = func() time.Time { return now }
	return cache, &now
}

func Test_DescribeTypeCache_ReusesResultWithinTTL(t *testing.T) {
	t.Parallel()

	client := &fakeCloudFormationClient{}
	cache, now := newTestDescribeTypeCache(client, time.Minute)

	first, err := cache.DescribeType(t.Context(), describeTypeInput("AWS::Logs::LogGroup"), withRegion("us-west-2"))
	require.NoError(t, err)

	*now = now.Add(59 * time.Second)
	second, err := cache.DescribeType(t.Context(), describeTypeInput("AWS::Logs::LogGroup"), withRegion("us-west-2"))
	require.NoError(t, err)

	require.Same(t, first, second)
	require.Equal(t, int32(1), client.calls.Load())
}

func Test_DescribeTypeCache_RefreshesAfterTTL(t *testing.T) {
	t.Parallel()

	client := &fakeCloudFormationClient{}
	cache, now := newTestDescribeTypeCache(client, time.Minute)

	_, err := cache.DescribeType(t.Context(), describeTypeInput("AWS::Logs::LogGroup"), withRegion("us-west-2"))
	require.NoError(t, err)

	*now = now.Add(time.Minute)
	_, err = cache.DescribeType(t.Context(), describeTypeInput("AWS::Logs::LogGroup"), withRegion("us-west-2"))
	require.NoError(t, err)

	require.Equal(t, int32(2), client.calls.Load())
}

func Test_DescribeTypeCache_SeparatesEntriesByTypeAndRegion(t *testing.T) {
	t.Parallel()

	client := &fakeCloudFormationClient{}
	cache, _ := newTestDescribeTypeCache(client, time.Minute)

	requests := []struct {
		typeName string
		region   string
	}{
		{typeName: "AWS::Logs::LogGroup", region: "us-west-2"},
		{typeName: "AWS::Logs::MetricFilter", region: "us-west-2"},
		{typeName: "AWS::Logs::LogGroup", region: "us-east-1"},
	}

	for _, r := range requests {
		output, err := cache.DescribeType(t.Context(), describeTypeInput(r.typeName), withRegion(r.region))
		require.NoError(t, err)
		require.Equal(t, r.region+"/"+r.typeName, aws.ToString(output.Schema))
	}
	require.Equal(t, int32(len(requests)), client.calls.Load())

	for _, r := range requests {
		output, err := cache.DescribeType(t.Context(), describeTypeInput(r.typeName), withRegion(r.region))
		require.NoError(t, err)
		require.Equal(t, r.region+"/"+r.typeName, aws.ToString(output.Schema))
	}
	require.Equal(t, int32(len(requests)), client.calls.Load())
}

func Test_DescribeTypeCache_DoesNotCacheErrors(t *testing.T) {
	t.Parallel()

	client := &fakeCloudFormationClient{err: errors.New("throttled")}
	cache, _ := newTestDescribeTypeCache(client, time.Minute)

	_, err := cache.DescribeType(t.Context(), describeTypeInput("AWS::Logs::LogGroup"), withRegion("us-west-2"))
	require.ErrorIs(t, err, client.err)

	client.err = nil
	output, err := cache.DescribeType(t.Context(), describeTypeInput("AWS::Logs::LogGroup"), withRegion("us-west-2"))
	require.NoError(t, err)
	require.NotNil(t, output)
	require.Equal(t, int32(2), client.calls.Load())
}

func Test_DescribeTypeCache_ConcurrentUse(t *testing.T) {
	t.Parallel()

	client := &fakeCloudFormationClient{}
	cache := NewDescribeTypeCache(client, time.Minute)
	typeNames := []string{"AWS::Logs::LogGroup", "AWS::Logs::MetricFilter", "AWS::S3::Bucket"}

	var wg sync.WaitGroup
	for i := range 50 {
		typeName := typeNames[i%len(typeNames)]
		wg.Go(func() {
			output, err := cache.DescribeType(t.Context(), describeTypeInput(typeName), withRegion("us-west-2"))
			assert.NoError(t, err)
			assert.Equal(t, "us-west-2/"+typeName, aws.ToString(output.Schema))
		})
	}
	wg.Wait()

	calls := client.calls.Load()
	for _, typeName := range typeNames {
		_, err := cache.DescribeType(t.Context(), describeTypeInput(typeName), withRegion("us-west-2"))
		require.NoError(t, err)
	}
	require.Equal(t, calls, client.calls.Load())
}
