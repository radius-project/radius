// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

// Package etcdstore stores resources using ETCd. Our usage for ETCd is optimized for the kinds
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
// 		scope|ucp:/planes/radius/local/resourceGroups/cool-group/|/Applications.Core/applications/cool-app/
// 		resource|ucp:/planes/radius/local/resourceGroups/cool-group/|/Applications.Core/applications/cool-app/
//
// scope or resource prefix helps with querying for scope or resources selectively.
// For example, a scope Query for planes would match all key prefixes such as scope:/ucp:/planes thus returning a
// list of planes and other "scopes" where as a resource query on planes would match all the resources under all the planes,
// which will be identifiable by key prefix resource|ucp:/planes
// Without the help of this prefix, when we query for a prefix ucp:/planes, we are potentially requesting for
// everything in the database
// The routing-scope is separated from the resource-path by the '|' separator.
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

func (c *ETCDClient) Query(ctx context.Context, query store.Query, options ...store.QueryOptions) ([]store.Object, error) {
	if ctx == nil {
		return nil, &store.ErrInvalid{Message: "invalid argument. 'ctx' is required"}
	}
	if query.RootScope == "" {
		return nil, &store.ErrInvalid{Message: "invalid argument. 'query.RootScope' is required"}
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

	values := []store.Object{}
	for _, kv := range response.Kvs {
		if keyMatchesQuery(kv.Key, query) {
			value := store.Object{}
			err = json.Unmarshal(kv.Value, &value)
			if err != nil {
				return nil, err
			}

			value.ETag = etag.NewFromRevision(kv.ModRevision)
			values = append(values, value)
		}
	}

	return values, nil
}

func (c *ETCDClient) Get(ctx context.Context, id resources.ID, options ...store.GetOptions) (*store.Object, error) {
	if ctx == nil {
		return nil, &store.ErrInvalid{Message: "invalid argument. 'ctx' is required"}
	}
	if id.IsEmpty() {
		return nil, &store.ErrInvalid{Message: "invalid argument. 'id' must not be empty"}
	}
	if id.IsCollection() {
		return nil, &store.ErrInvalid{Message: "invalid argument. 'id' must refer to a named resource, not a collection"}
	}

	key := keyFromID(id)
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

func (c *ETCDClient) Delete(ctx context.Context, id resources.ID, options ...store.DeleteOptions) error {
	if ctx == nil {
		return &store.ErrInvalid{Message: "invalid argument. 'ctx' is required"}
	}
	if id.IsEmpty() {
		return &store.ErrInvalid{Message: "invalid argument. 'id' must not be empty"}
	}
	if id.IsCollection() {
		return &store.ErrInvalid{Message: "invalid argument. 'id' must refer to a named resource, not a collection"}
	}

	key := keyFromID(id)
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

func idFromKey(key []byte) (resources.ID, error) {
	parts := strings.Split(string(key), SectionSeparator)
	// sample valid key:
	// scope|ucp:/planes/radius/local/resourceGroups/cool-group/|/Applications.Core/applications/cool-app/
	if len(parts) != 3 {
		return resources.ID{}, errors.New("the ETCd key '%q' is invalid because it does not have 3 sections")
	}

	if parts[2] == "" {
		// Scope reference
		parsed, err := resources.Parse(parts[1])
		if err != nil {
			return resources.ID{}, err
		}

		return parsed, nil
	}

	// The key might look like:
	// 	scope|ucp:/planes/radius/local/resourceGroups/cool-group/|/Applications.Core/applications/cool-app/
	// OR
	// 	/subscriptions/{guid}/resourceGroups/cool-group/|/Applications.Core/applications/cool-app/
	//
	// We put it back together by adding "providers" and then it's a valid resource ID.
	parsed, err := resources.Parse(parts[1] + resources.ProvidersSegment + parts[2])
	if err != nil {
		return resources.ID{}, err
	}

	return parsed, nil
}

// keyFromID returns the key to use for an ID. They key should be used as an exact match.
func keyFromID(id resources.ID) string {
	var scopeOrResource = store.UCPResourcePrefix
	if id.IsScope() {
		scopeOrResource = store.UCPScopePrefix
	}

	return scopeOrResource + SectionSeparator + normalize(id.RootScope()) +
		SectionSeparator + normalize(id.RoutingScope())
}

// keyFromQuery returns the key to use for an ID. The key should be used as a prefix.
func keyFromQuery(query store.Query) string {
	if query.ScopeRecursive {
		// A recursive query will not be able to consider anything in the routing scope, so it
		// always requires client-side filtering.
		key := normalize(query.RootScope)
		if query.IsScopeQuery {
			key = store.UCPScopePrefix + SectionSeparator + key
		} else {
			key = store.UCPResourcePrefix + SectionSeparator + key
		}
		return key
	}

	key := normalize(query.RootScope) + SectionSeparator + normalize(query.RoutingScopePrefix)
	if query.IsScopeQuery {
		key = store.UCPScopePrefix + SectionSeparator + key
	} else {
		key = store.UCPResourcePrefix + SectionSeparator + key
	}
	return key
}

func keyMatchesQuery(key []byte, query store.Query) bool {
	// The only case we have to filter explicitly here is when a Resource Type filter is applied
	// or for a scope recursive query. The rest of these cases have their logic just handled by the key
	// mechanism.
	if !query.ScopeRecursive && query.ResourceType == "" {
		return true
	}

	// OK we have to filter.

	// Ignore invalid keys, we don't expect to find them.
	id, err := idFromKey(key)
	if err != nil {
		return false
	}

	if query.RoutingScopePrefix != "" && !strings.HasPrefix(normalize(id.RoutingScope()), normalize(query.RoutingScopePrefix)) {
		return false // Not a match for the routing scope.
	}

	if query.ResourceType != "" && !strings.EqualFold(id.Type(), query.ResourceType) {
		return false // Not a match for the resource type
	}

	return true
}

func normalize(part string) string {
	if len(part) == 0 {
		return ""
	}
	if strings.HasPrefix(part, resources.UCPPrefix+resources.SegmentSeparator) {
		// Already prefixed
	} else if !strings.HasPrefix(part, resources.SegmentSeparator) {
		part = resources.SegmentSeparator + part
	}
	if !strings.HasSuffix(part, resources.SegmentSeparator) {
		part = part + resources.SegmentSeparator
	}

	return strings.ToLower(part)
}
