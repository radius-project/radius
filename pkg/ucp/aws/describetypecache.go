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
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
)

// DefaultDescribeTypeCacheTTL is how long a cached DescribeType result is reused before it is fetched again.
const DefaultDescribeTypeCacheTTL = 15 * time.Minute

var _ AWSCloudFormationClient = (*DescribeTypeCache)(nil)

// DescribeTypeCache is an AWSCloudFormationClient that caches successful DescribeType results.
// UCP calls DescribeType on every AWS resource request to read the resource type schema, and the
// CloudFormation API has a low rate limit, so concurrent deployments can be throttled without a cache.
// It is safe for concurrent use. Callers must not modify the returned output.
type DescribeTypeCache struct {
	client AWSCloudFormationClient
	ttl    time.Duration
	now    func() time.Time

	mu      sync.RWMutex
	entries map[describeTypeCacheKey]describeTypeCacheEntry
}

type describeTypeCacheKey struct {
	region    string
	typ       string
	typeName  string
	arn       string
	versionID string
}

type describeTypeCacheEntry struct {
	output    *cloudformation.DescribeTypeOutput
	expiresAt time.Time
}

// NewDescribeTypeCache returns a DescribeTypeCache that wraps client and keeps results for ttl.
func NewDescribeTypeCache(client AWSCloudFormationClient, ttl time.Duration) *DescribeTypeCache {
	return &DescribeTypeCache{
		client:  client,
		ttl:     ttl,
		now:     time.Now,
		entries: map[describeTypeCacheKey]describeTypeCacheEntry{},
	}
}

// DescribeType returns the cached result for the request if it has not expired, and otherwise calls
// the wrapped client and caches a successful result. Errors are never cached.
func (c *DescribeTypeCache) DescribeType(ctx context.Context, params *cloudformation.DescribeTypeInput, optFns ...func(*cloudformation.Options)) (*cloudformation.DescribeTypeOutput, error) {
	if params == nil {
		return c.client.DescribeType(ctx, params, optFns...)
	}

	key := newDescribeTypeCacheKey(params, optFns)

	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	if ok && c.now().Before(entry.expiresAt) {
		return entry.output, nil
	}

	output, err := c.client.DescribeType(ctx, params, optFns...)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.entries[key] = describeTypeCacheEntry{output: output, expiresAt: c.now().Add(c.ttl)}
	c.mu.Unlock()

	return output, nil
}

func newDescribeTypeCacheKey(params *cloudformation.DescribeTypeInput, optFns []func(*cloudformation.Options)) describeTypeCacheKey {
	// The region is set through optFns, so apply them to an empty Options to read it.
	opts := cloudformation.Options{}
	for _, fn := range optFns {
		fn(&opts)
	}

	return describeTypeCacheKey{
		region:    opts.Region,
		typ:       string(params.Type),
		typeName:  aws.ToString(params.TypeName),
		arn:       aws.ToString(params.Arn),
		versionID: aws.ToString(params.VersionId),
	}
}
