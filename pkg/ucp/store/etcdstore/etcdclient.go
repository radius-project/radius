/*
------------------------------------------------------------
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
------------------------------------------------------------
*/

// Package etcdstore stores resources using etcd. Our usage for etcd is optimized for the kinds
// of hierarchical and type-based queries common in a resource provider.
//
// Our key prefix scheme builds a hierarchy using '|' as a separator as '|' is illegal in an
// UCP resource identifier. We take advantage of the natural usage of '/' in resource ids as
// a delimiter.
//
// The key of a resource can be mechanically constructed from its resource id by replacing 'providers'
// with the '|' separator (for a non-extension resource). We have no current support for extension resources.
//
// Keys are structured like the following example:
//
//	scope|/planes/radius/local/|/resourceGroups/cool-group/
//	resource|/planes/radius/local/resourceGroups/cool-group/|/Applications.Core/applications/cool-app/
//
// As a special case for scopes (like resource groups) we treat the last segment as the routing scope.
//
// The prefix (scope or resource) limits each query to either for scope or resources respectively. In our
// use cases for the store we never need to query scopes and resources at the same time. Separating these actions
// limits the number of results - we want to avoid cases where the query has to return a huge set of results.
//
// For example, the following query will be commonly executed and we don't want it to list all resources in the
// database:
//
//	scope|/planes/
//
// This scheme allows a variety of flexibility for querying/filtering with different scopes. We prefer
// query approaches that that involved client-side filtering to avoid the need for N+1 query strategies.
// Leading and trailing '/' characters are preserved on the key-segments to avoid ambiguity.
package etcdstore

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/store/storeutil"
	"github.com/project-radius/radius/pkg/ucp/util/etag"
	etcdclient "go.etcd.io/etcd/client/v3"
)

const (
	SectionSeparator = "|"
)

func NewETCDClient(c *etcdclient.Client) *ETCDClient {
	return &ETCDClient{client: c}
}

var _ store.StorageClient = (*ETCDClient)(nil)

type ETCDClient struct {
	client *etcdclient.Client
}

func (c *ETCDClient) Query(ctx context.Context, query store.Query, options ...store.QueryOptions) (*store.ObjectQueryResult, error) {
	if ctx == nil {
		return nil, &store.ErrInvalid{Message: "invalid argument. 'ctx' is required"}
	}
	if query.RootScope == "" {
		return nil, &store.ErrInvalid{Message: "invalid argument. 'query.RootScope' is required"}
	}
	if query.IsScopeQuery && query.RoutingScopePrefix != "" {
		return nil, &store.ErrInvalid{Message: "invalid argument. 'query.RoutingScopePrefix' is not supported for scope queries"}
	}

	key := keyFromQuery(query)

	// TODO: We don't place a limit/top value on the query right now so we get all
	// results as a single page. This would be a nice future improvement
	//
	// https://stackoverflow.com/questions/44873514/etcd3-go-client-how-to-paginate-large-sets-of-keys
	response, err := c.client.Get(ctx, key, etcdclient.WithPrefix())
	if err != nil {
		return nil, err
	}

	results := store.ObjectQueryResult{}
	for _, kv := range response.Kvs {
		if keyMatchesQuery(kv.Key, query) {
			value := store.Object{}
			err = json.Unmarshal(kv.Value, &value)
			if err != nil {
				return nil, err
			}

			match, err := value.MatchesFilters(query.Filters)
			if err != nil {
				return nil, err
			} else if !match {
				continue
			}

			value.ETag = etag.NewFromRevision(kv.ModRevision)
			results.Items = append(results.Items, value)
		}
	}

	return &results, nil
}

func (c *ETCDClient) Get(ctx context.Context, id string, options ...store.GetOptions) (*store.Object, error) {
	if ctx == nil {
		return nil, &store.ErrInvalid{Message: "invalid argument. 'ctx' is required"}
	}
	parsed, err := resources.Parse(id)
	if err != nil {
		return nil, &store.ErrInvalid{Message: "invalid argument. 'id' must be a valid resource id"}
	}
	if parsed.IsEmpty() {
		return nil, &store.ErrInvalid{Message: "invalid argument. 'id' must not be empty"}
	}
	if parsed.IsResourceCollection() || parsed.IsScopeCollection() {
		return nil, &store.ErrInvalid{Message: "invalid argument. 'id' must refer to a named resource, not a collection"}
	}

	key := keyFromID(parsed)
	response, err := c.client.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	if response.Count == 0 {
		return nil, &store.ErrNotFound{}
	}

	value := store.Object{}
	err = json.Unmarshal(response.Kvs[0].Value, &value)
	if err != nil {
		return nil, err
	}

	value.ETag = etag.NewFromRevision(response.Kvs[0].ModRevision)

	return &value, nil
}

func (c *ETCDClient) Delete(ctx context.Context, id string, options ...store.DeleteOptions) error {
	if ctx == nil {
		return &store.ErrInvalid{Message: "invalid argument. 'ctx' is required"}
	}
	parsed, err := resources.Parse(id)
	if err != nil {
		return &store.ErrInvalid{Message: "invalid argument. 'id' must be a valid resource id"}
	}
	if parsed.IsEmpty() {
		return &store.ErrInvalid{Message: "invalid argument. 'id' must not be empty"}
	}
	if parsed.IsResourceCollection() || parsed.IsScopeCollection() {
		return &store.ErrInvalid{Message: "invalid argument. 'id' must refer to a named resource, not a collection"}
	}

	key := keyFromID(parsed)
	config := store.NewDeleteConfig(options...)

	// If we have an ETag then we do to execute a transaction.
	if config.ETag != "" {
		revision, err := etag.ParseRevision(config.ETag)
		if err != nil {
			// Treat an invalid ETag as a concurrency failure, since it will never match.
			return &store.ErrConcurrency{}
		}

		txn, err := c.client.Txn(ctx).
			If(etcdclient.Compare(etcdclient.ModRevision(key), "=", revision)).
			Then(etcdclient.OpDelete(key)).
			Commit()
		if err != nil {
			return err
		}

		if !txn.Succeeded {
			return &store.ErrConcurrency{}
		}

		response := txn.Responses[0].GetResponseDeleteRange()
		if response.Deleted == 0 {
			return &store.ErrNotFound{}
		} else {
			return nil
		}
	}

	// If we don't have an ETag then things are straightforward :)
	response, err := c.client.Delete(ctx, key)
	if err != nil {
		return err
	}

	if response.Deleted == 0 {
		return &store.ErrNotFound{}
	}

	return nil
}

func (c *ETCDClient) Save(ctx context.Context, obj *store.Object, options ...store.SaveOptions) error {
	if ctx == nil {
		return &store.ErrInvalid{Message: "invalid argument. 'ctx' is required"}
	}
	if obj == nil {
		return &store.ErrInvalid{Message: "invalid argument. 'obj' is required"}
	}

	id := obj.Metadata.ID
	parsed, err := resources.Parse(id)
	if err != nil {
		return err
	}

	b, err := json.Marshal(obj)
	if err != nil {
		return err
	}

	key := keyFromID(parsed)
	config := store.NewSaveConfig(options...)

	// If we have an ETag then we do to execute a transaction.
	if config.ETag != "" {
		revision, err := etag.ParseRevision(config.ETag)
		if err != nil {
			// Treat an invalid ETag as a concurrency failure, since it will never match.
			return &store.ErrConcurrency{}
		}

		txn, err := c.client.Txn(ctx).
			If(etcdclient.Compare(etcdclient.ModRevision(key), "=", revision)).
			Then(etcdclient.OpPut(key, string(b))).
			Commit()
		if err != nil {
			return err
		}

		if !txn.Succeeded {
			return &store.ErrConcurrency{}
		}

		response := txn.Responses[0].GetResponsePut()
		obj.ETag = etag.NewFromRevision(response.Header.Revision)
		return nil
	}

	// If we don't have an ETag then things are pretty straightforward.
	response, err := c.client.Put(ctx, key, string(b))
	if err != nil {
		return err
	}

	obj.ETag = etag.NewFromRevision(response.Header.Revision)

	return nil
}

func (c *ETCDClient) Client() *etcdclient.Client {
	return c.client
}

func idFromKey(key []byte) (resources.ID, error) {
	parts := strings.Split(string(key), SectionSeparator)
	// sample valid key:
	// scope|/planes/radius/local/resourceGroups/cool-group/|/Applications.Core/applications/cool-app/
	if len(parts) != 3 {
		return resources.ID{}, errors.New("the etcd key '%q' is invalid because it does not have 3 sections")
	}

	switch parts[0] {
	case storeutil.ScopePrefix:
		// The key might look like:
		//		scope|/planes/radius/local/|/resourceGroups/cool-group/
		return resources.Parse(parts[1] + strings.TrimPrefix(parts[2], resources.SegmentSeparator))

	case storeutil.ResourcePrefix:
		// The key might look like:
		//		resource|/subscriptions/{guid}/resourceGroups/cool-group/|/Applications.Core/applications/cool-app/
		return resources.Parse(parts[1] + resources.ProvidersSegment + parts[2])

	default:
		return resources.ID{}, errors.New("the etcd key '%q' is invalid because it has the wrong prefix")
	}
}

// keyFromID returns the key to use for an ID. They key should be used as an exact match.
func keyFromID(id resources.ID) string {
	prefix, rootScope, routingScope, _ := storeutil.ExtractStorageParts(id)
	return prefix + SectionSeparator + rootScope + SectionSeparator + routingScope
}

// keyFromQuery returns the key to use for an for executing a query. The key should be used as a prefix.
func keyFromQuery(query store.Query) string {
	// These patterns require a prefix match for us in ETCd.
	//
	// A recursive query will not be able to consider anything in the routing scope, so it
	// always requires client-side filtering.
	prefix := storeutil.ResourcePrefix
	if query.IsScopeQuery {
		prefix = storeutil.ScopePrefix
	}

	if query.ScopeRecursive {
		return prefix + SectionSeparator + storeutil.NormalizePart(query.RootScope)
	} else {
		return prefix + SectionSeparator + storeutil.NormalizePart(query.RootScope) + SectionSeparator + storeutil.NormalizePart(query.RoutingScopePrefix)
	}
}

func keyMatchesQuery(key []byte, query store.Query) bool {
	// Ignore invalid keys, we don't expect to find them.
	id, err := idFromKey(key)
	if err != nil {
		return false
	}

	return storeutil.IDMatchesQuery(id, query)
}
