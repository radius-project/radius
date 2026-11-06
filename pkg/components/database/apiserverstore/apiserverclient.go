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

// Package apiserverstore stores resources using the Kubernetes API Server - using CRDs as a key-value store.
// We don't represent UCP data directly as Kubernetes resources because that would require the creation of
// many types. The complex hierarchies of data that are possible for UCP aren't a good fit for Kubernetes
// data model.
//
// Our strategy is to use the resource name and hash of the object name in order to derive a *likely*-unique
// kubernetes object name. Then we affix labels to the object that match its scopes so we can easily author queries.
//
// Since this scheme allows collisions we need to use optimistic concurrency controls when writing and
// consider the possibility of multiple resources being present when reading.
//
// Each Kubernetes Resource object stores a list of UCP resources. Since we use truncated SHA-256 hashes,
// we expect collisions to be extremely rare. The only case we need to be concerned about is when collisions
// cause the total size of the Kubernetes Resource object to be larger than the 8mb limit of Kubernetes.
//
// This scheme allows us to perform O(1) reads and writes for key-based lookups while still handling
// collisions.
//
// The kubernetes resource names we use are built according to the following format:
//
//	<resource name>.<id hash>
//
// We also use a labeling scheme to attach each root scope segment and the resource type as a label to the
// Kubernetes objects. This allows us to filter the number of objects we transact with using the labels as hints.
package apiserverstore

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/radius-project/radius/pkg/components/database"
	ucpv1alpha1 "github.com/radius-project/radius/pkg/components/database/apiserverstore/api/ucp.dev/v1alpha1"
	"github.com/radius-project/radius/pkg/components/database/databaseutil"
	"github.com/radius-project/radius/pkg/hashutil"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	"github.com/radius-project/radius/pkg/ucp/util/etag"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/util/wait"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// LabelKind is used to determine whether an object holds scopes or resources. Conflicts are not possible due to the way we do naming.
	// Each Kubernetes object holds only scopes or only resources.
	LabelKind = "ucp.dev/kind"

	// LabelScopeFormat is used format a label that describes the scope. The placeholder is replaced by the scope type (eg: resourceGroup).
	LabelScopeFormat = "ucp.dev/scope-%s"

	// LabelResourceType is used as the key of a label describing the resource type.
	LabelResourceType = "ucp.dev/resource-type"

	// LabelValueMultiple is used as the label value when a resource matches multiple scopes or types due to
	// hash collision.
	LabelValueMultiple = "m_u_l_t_i_p_l_e"

	// RetryCount is the number of retries we will make on optimistic concurrency failures. The need for retries is **rare** because
	// it only happens on concurrent operations to the same UCP resource or on a hash collision.
	RetryCount = 10
)

// NewAPIServerClient creates a new APIServerClient object which is used to interact with the API server.
func NewAPIServerClient(client runtimeclient.Client, namespace string) *APIServerClient {
	return &APIServerClient{client: client, namespace: namespace}
}

var _ database.Client = (*APIServerClient)(nil)

type APIServerClient struct {
	client    runtimeclient.Client
	namespace string

	// readyChan is used for testing concurrency behavior. This will be signaled when the client is ready to perform a write.
	readyChan chan<- struct{}

	// waitChan is used for testing concurrency behavior. This will be read from before the client performs a write.
	waitChan <-chan struct{}
}

// Query searches for objects in the store that match the given query and returns them.
func (c *APIServerClient) Query(ctx context.Context, query database.Query, options ...database.QueryOptions) (*database.ObjectQueryResult, error) {
	if ctx == nil {
		return nil, &database.ErrInvalid{Message: "invalid argument. 'ctx' is required"}
	}
	err := query.Validate()
	if err != nil {
		return nil, &database.ErrInvalid{Message: fmt.Sprintf("invalid argument. Query is invalid: %s", err.Error())}
	}

	selector, err := createLabelSelector(query)
	if err != nil {
		return nil, err
	}

	rs := ucpv1alpha1.ResourceList{}
	err = c.list(ctx, &rs, runtimeclient.InNamespace(c.namespace), runtimeclient.MatchingLabelsSelector{Selector: selector})
	if err != nil {
		return nil, err
	}

	// A resource can be stored under both its current (SHA-256) object name and its legacy (SHA-1) object name, for
	// example while it is being moved or after an older writer created a second copy. Reads return the current copy,
	// so a query selects that copy first and then applies filters: a stale legacy copy must not match a filter that
	// the copy reads return does not match.
	type selection struct {
		entry   *ucpv1alpha1.ResourceEntry
		current bool
	}
	order := []string{}
	selected := map[string]selection{}
	for i := range rs.Items {
		resource := &rs.Items[i]
		for j := range resource.Entries {
			entry := &resource.Entries[j]
			id, err := resources.Parse(entry.ID)
			if err != nil {
				// Ignore invalid IDs when querying, we don't want a single piece of bad data to
				// break all queries.
				logger := ucplog.FromContextOrDiscard(ctx)
				logger.Error(err, "found an invalid resource id as part of a query", "name", resource.Name, "namespace", resource.Namespace)
				continue
			}

			if !databaseutil.IDMatchesQuery(id, query) {
				continue
			}

			key := strings.ToLower(entry.ID)
			current := resource.Name == resourceName(id)
			if existing, ok := selected[key]; ok {
				if current && !existing.current {
					selected[key] = selection{entry: entry, current: true}
				}
				continue
			}

			order = append(order, key)
			selected[key] = selection{entry: entry, current: current}
		}
	}

	results := database.ObjectQueryResult{}
	for _, key := range order {
		converted, err := readEntry(selected[key].entry)
		if err != nil {
			return nil, err
		}

		match, err := converted.MatchesFilters(query.Filters)
		if err != nil {
			return nil, err
		} else if !match {
			continue
		}

		results.Items = append(results.Items, *converted)
	}

	return &results, nil
}

// Get retrieves an object from the store given its ID, or returns an error if the object does not exist or if an error occurs.
func (c *APIServerClient) Get(ctx context.Context, id string, options ...database.GetOptions) (*database.Object, error) {
	if ctx == nil {
		return nil, &database.ErrInvalid{Message: "invalid argument. 'ctx' is required"}
	}
	parsed, err := resources.Parse(id)
	if err != nil {
		return nil, &database.ErrInvalid{Message: "invalid argument. 'id' must be a valid resource id"}
	}
	if parsed.IsEmpty() {
		return nil, &database.ErrInvalid{Message: "invalid argument. 'id' must not be empty"}
	}
	if parsed.IsResourceCollection() || parsed.IsScopeCollection() {
		return nil, &database.ErrInvalid{Message: "invalid argument. 'id' must refer to a named resource, not a collection"}
	}

	resource := ucpv1alpha1.Resource{}
	err = c.getResourceWithFallback(ctx, parsed, &resource)
	if err != nil && apierrors.IsNotFound(err) {
		return nil, &database.ErrNotFound{ID: id}
	} else if err != nil {
		return nil, err
	}

	obj, err := read(&resource, parsed)
	if err != nil {
		return nil, err
	} else if obj == nil {
		return nil, &database.ErrNotFound{ID: id}
	}

	return obj, nil
}

// Delete deletes a resource from the store, returning an error if the resource does not exist or if there is a concurrency conflict.
func (c *APIServerClient) Delete(ctx context.Context, id string, options ...database.DeleteOptions) error {
	if ctx == nil {
		return &database.ErrInvalid{Message: "invalid argument. 'ctx' is required"}
	}
	parsed, err := resources.Parse(id)
	if err != nil {
		return &database.ErrInvalid{Message: "invalid argument. 'id' must be a valid resource id"}
	}
	if parsed.IsEmpty() {
		return &database.ErrInvalid{Message: "invalid argument. 'id' must not be empty"}
	}
	if parsed.IsResourceCollection() || parsed.IsScopeCollection() {
		return &database.ErrInvalid{Message: "invalid argument. 'id' must refer to a named resource, not a collection"}
	}

	config := database.NewDeleteConfig(options...)

	// removed holds the ETags of the copies this delete removed. A later attempt that finds no copy succeeds, and one
	// that finds a copy of a removed value removes it without checking the ETag again. removedLegacy records that one of
	// them was the legacy copy.
	removed := map[string]bool{}
	removedLegacy := false
	err = c.doWithRetry(func() (bool, error) {
		copies, exists, err := c.storedCopies(ctx, parsed)
		if err != nil {
			return false, err
		}

		if len(copies) == 0 {
			switch {
			case len(removed) > 0:
				// The remaining copy was removed concurrently.
				return false, nil
			case !exists && config.ETag != "":
				return false, &database.ErrConcurrency{}
			default:
				return false, &database.ErrNotFound{ID: id}
			}
		}

		// Reads return the first copy, so it decides whether the ETag matches.
		first := copies[0].Entries[*findIndex(&copies[0], parsed)].ETag
		if config.ETag != "" && config.ETag != first && !removed[first] {
			return false, &database.ErrConcurrency{}
		}

		c.synchronize()

		// Every copy is removed, so a read cannot fall back to a copy the delete left behind. The copy reads return is
		// removed last, so a concurrent read finds the resource until the delete completes.
		for i := len(copies) - 1; i >= 0; i-- {
			index := *findIndex(&copies[i], parsed)
			etag := copies[i].Entries[index].ETag
			err := c.removeEntry(ctx, &copies[i], index)
			if apierrors.IsConflict(err) || apierrors.IsNotFound(err) {
				return true, err // RETRY this!
			} else if err != nil {
				return false, err
			}

			removed[etag] = true
			removedLegacy = removedLegacy || copies[i].Name == legacyResourceName(parsed)
		}

		if !removedLegacy {
			return false, nil
		}

		return c.copyOfRemoved(ctx, parsed, removed)
	})

	return err
}

// copyOfRemoved returns a retryable error if the current object stores a copy of a value in removed. The migration
// writes such a copy when it read the legacy copy before a delete removed it, and the write can land after the delete
// read the current object. The migration removes that copy itself when it finds the legacy copy gone, but it may stop
// before it does, so the delete checks again.
func (c *APIServerClient) copyOfRemoved(ctx context.Context, id resources.ID, removed map[string]bool) (bool, error) {
	current := ucpv1alpha1.Resource{}
	err := c.get(ctx, runtimeclient.ObjectKey{Namespace: c.namespace, Name: resourceName(id)}, &current)
	if apierrors.IsNotFound(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}

	if index := findIndex(&current, id); index != nil && removed[current.Entries[*index].ETag] {
		return true, &database.ErrConcurrency{}
	}

	return false, nil
}

// storedCopies returns the Kubernetes objects that store an entry for id, the current (SHA-256) object first. An entry
// has two copies while it is moved to its current object name, after an interrupted move, or when an older writer
// created a second copy. exists reports whether either object exists.
func (c *APIServerClient) storedCopies(ctx context.Context, id resources.ID) ([]ucpv1alpha1.Resource, bool, error) {
	copies := []ucpv1alpha1.Resource{}
	exists := false
	for _, name := range []string{resourceName(id), legacyResourceName(id)} {
		resource := ucpv1alpha1.Resource{}
		err := c.get(ctx, runtimeclient.ObjectKey{Namespace: c.namespace, Name: name}, &resource)
		if apierrors.IsNotFound(err) {
			continue
		} else if err != nil {
			return nil, false, err
		}

		exists = true
		if findIndex(&resource, id) != nil {
			copies = append(copies, resource)
		}
	}

	if len(copies) > 0 {
		return copies, exists, nil
	}

	// The migration may have moved the entry from the legacy object to the current object between the two reads. It
	// writes the current copy before it removes the legacy copy, so one more read of the current object finds it.
	current := ucpv1alpha1.Resource{}
	err := c.get(ctx, runtimeclient.ObjectKey{Namespace: c.namespace, Name: resourceName(id)}, &current)
	if apierrors.IsNotFound(err) {
		return copies, exists, nil
	} else if err != nil {
		return nil, false, err
	}

	if findIndex(&current, id) != nil {
		copies = append(copies, current)
	}

	return copies, true, nil
}

// Save saves an object to the store, or updates an existing object if it already exists, and returns an error if the operation fails.
func (c *APIServerClient) Save(ctx context.Context, obj *database.Object, options ...database.SaveOptions) error {
	if ctx == nil {
		return &database.ErrInvalid{Message: "invalid argument. 'ctx' is required"}
	}
	if obj == nil {
		return &database.ErrInvalid{Message: "invalid argument. 'obj' is required"}
	}

	id, err := resources.Parse(obj.ID)
	if err != nil {
		return err
	}

	config := database.NewSaveConfig(options...)
	if config.CreateOnly && config.ETag != "" {
		return &database.ErrInvalid{Message: "invalid argument. 'CreateOnly' cannot be combined with an ETag"}
	}

	// replaced and written are the entry a write to the legacy object of the entry replaced, and the entry it wrote.
	var replaced, written *ucpv1alpha1.ResourceEntry
	err = c.doWithRetry(func() (bool, error) {
		replaced, written = nil, nil
		name, resource, found, err := c.getResourceForSave(ctx, id)
		if err != nil {
			return false, err
		}

		// These need to be initialized if we're creating the object.
		resource.Name = name
		resource.Namespace = c.namespace

		converted, err := convert(obj)
		if err != nil {
			return false, err
		}

		// Set the ETag so the caller can see the computed value.
		obj.ETag = converted.ETag

		index := findIndex(&resource, id)
		if index != nil && config.CreateOnly {
			return false, &database.ErrConcurrency{}
		} else if index == nil && config.ETag != "" {
			// The ETag is only meaning for a replace/update operation not a create. We treat
			// the absence of the resource as a match failure.
			return false, &database.ErrConcurrency{}
		} else if index == nil {
			resource.Entries = append(resource.Entries, *converted)
		} else {
			if config.ETag != "" && config.ETag != resource.Entries[*index].ETag {
				return false, &database.ErrConcurrency{}
			}

			if name == legacyResourceName(id) {
				replaced, written = resource.Entries[*index].DeepCopy(), converted.DeepCopy()
			}
			resource.Entries[*index] = *converted
		}

		resource.Labels = assignLabels(&resource)

		c.synchronize()

		if found {
			err = c.client.Update(ctx, &resource)
			if err != nil && apierrors.IsConflict(err) {
				return true, err // Retry this!
			} else if err != nil {
				return false, err
			}
		} else {
			err = c.client.Create(ctx, &resource)
			if err != nil && apierrors.IsConflict(err) {
				return true, err // Retry this!
			} else if err != nil && apierrors.IsAlreadyExists(err) {
				return true, err // Retry this!
			} else if err != nil {
				return false, err
			}
		}

		return false, nil
	})
	if err != nil || written == nil {
		return err
	}

	return c.reconcileLegacyWrite(ctx, id, *replaced, *written, config.ETag != "")
}

// reconcileLegacyWrite makes a write to the legacy object of an entry visible, or undoes it. A save writes to the
// legacy object when the entry was not in its current object, but the migration can copy the entry there before the
// write. Reads then return the copy, and the migration leaves the two different copies in place as a conflict, so the
// write would be lost.
//
// If the current object holds the value the save replaced, it is a copy that is now stale, and it is updated to the
// written value. If it holds another value, another writer saved it to the copy, and the write to the legacy object is
// removed: a conditional save then fails as if it had read the copy, and an unconditional save is ordered before the
// other write. A write that a later write to the legacy object replaced is left to it.
func (c *APIServerClient) reconcileLegacyWrite(ctx context.Context, id resources.ID, replaced ucpv1alpha1.ResourceEntry, written ucpv1alpha1.ResourceEntry, conditional bool) error {
	currentKey := runtimeclient.ObjectKey{Namespace: c.namespace, Name: resourceName(id)}
	for range RetryCount {
		current := ucpv1alpha1.Resource{}
		err := c.get(ctx, currentKey, &current)
		if apierrors.IsNotFound(err) {
			return nil
		} else if err != nil {
			return err
		}

		index := findIndex(&current, id)
		switch {
		case index == nil || sameEntry(current.Entries[*index], written):
			// There is no copy, or the migration already copied the written value.
			return nil
		case sameEntry(current.Entries[*index], replaced):
			current.Entries[*index] = *written.DeepCopy()
			current.Labels = assignLabels(&current)
			err = c.client.Update(ctx, &current)
			if isRetryable(err) || apierrors.IsNotFound(err) {
				continue
			} else if err != nil {
				return err
			}

			return nil
		default:
			removed, err := c.removeLegacyWrite(ctx, id, written)
			if err != nil {
				return err
			} else if removed && conditional {
				return &database.ErrConcurrency{}
			}

			return nil
		}
	}

	return &database.ErrConcurrency{}
}

// removeLegacyWrite removes written from the legacy object of id, and reports whether it did. It leaves an entry that
// was written or removed since.
func (c *APIServerClient) removeLegacyWrite(ctx context.Context, id resources.ID, written ucpv1alpha1.ResourceEntry) (bool, error) {
	legacyKey := runtimeclient.ObjectKey{Namespace: c.namespace, Name: legacyResourceName(id)}
	for range RetryCount {
		legacy := ucpv1alpha1.Resource{}
		err := c.get(ctx, legacyKey, &legacy)
		if apierrors.IsNotFound(err) {
			return false, nil
		} else if err != nil {
			return false, err
		}

		index := findIndex(&legacy, id)
		if index == nil || legacy.Entries[*index].ETag != written.ETag {
			return false, nil
		}

		err = c.removeEntry(ctx, &legacy, *index)
		if isRetryable(err) || apierrors.IsNotFound(err) {
			continue
		} else if err != nil {
			return false, err
		}

		return true, nil
	}

	return false, &database.ErrConcurrency{}
}

// transientBackoff is the backoff used to retry reads that fail with a transient API server error.
var transientBackoff = wait.Backoff{Duration: 200 * time.Millisecond, Factor: 2, Steps: 5}

// isTransient reports whether err is a temporary API server failure (for example a 503 while the
// API server or an aggregated API is briefly unavailable) that is safe to retry.
func isTransient(err error) bool {
	return apierrors.IsServiceUnavailable(err) || apierrors.IsServerTimeout(err) || apierrors.IsTimeout(err) || apierrors.IsTooManyRequests(err)
}

// get reads an object and retries when the API server returns a transient error.
func (c *APIServerClient) get(ctx context.Context, key runtimeclient.ObjectKey, obj runtimeclient.Object) error {
	return retryRead(ctx, "get", key.Name, func() error {
		return c.client.Get(ctx, key, obj)
	})
}

// list lists objects and retries when the API server returns a transient error.
func (c *APIServerClient) list(ctx context.Context, list runtimeclient.ObjectList, opts ...runtimeclient.ListOption) error {
	return retryRead(ctx, "list", "", func() error {
		return c.client.List(ctx, list, opts...)
	})
}

// retryRead runs a read-only API server call, retrying transient errors with transientBackoff. It
// stops waiting as soon as ctx is cancelled and returns the last API error.
func retryRead(ctx context.Context, op string, name string, read func() error) error {
	backoff := transientBackoff
	for {
		err := read()
		if err == nil || !isTransient(err) || backoff.Steps <= 1 {
			return err
		}

		delay := backoff.Step()
		ucplog.FromContextOrDiscard(ctx).Info("Retrying Kubernetes read after transient error", "operation", op, "name", name, "delay", delay.String(), "error", err.Error())

		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return err
		case <-timer.C:
		}
	}
}
func (c *APIServerClient) doWithRetry(action func() (bool, error)) error {
	for range RetryCount {
		retryable, err := action()
		if err != nil && !retryable {
			return err
		}

		if err == nil {
			return nil
		}

		// Error was retryable.
	}

	// If we get here then we ran out of retries.
	return &database.ErrConcurrency{}
}

// synchronize is used for testing concurrency behavior. The client can be configured by tests to pause between reading and writing
// allowing the test to modify the underlying resources. This is how we test optimistic concurrency failures.
func (c *APIServerClient) synchronize() {
	if c.readyChan != nil {
		c.readyChan <- struct{}{}
	}

	if c.waitChan != nil {
		<-c.waitChan
	}
}

func normalizeName(name string) string {
	sb := strings.Builder{}
	for _, ch := range name {
		// Since this store uses . (dot) as a separator of the key, it converts dot character to hex code.
		if unicode.IsDigit(ch) || unicode.IsLetter(ch) || ch == '-' {
			sb.WriteRune(unicode.ToLower(ch))
		} else {
			sb.WriteString(fmt.Sprintf("x%02x", ch))
		}
	}

	return sb.String()
}

func resourceName(id resources.ID) string {
	return makeResourceName(id, hashutil.Hex([]byte(strings.ToLower(id.String()))))
}

// legacyResourceName computes the Kubernetes object name using the legacy SHA-1 hash.
//
// SHA-1 is retained only to locate resources written by older versions of Radius during the
// migration to SHA-256. Use resourceName for new values. See
// https://github.com/radius-project/radius/issues/8084.
func legacyResourceName(id resources.ID) string {
	return makeResourceName(id, hashutil.LegacyHex([]byte(strings.ToLower(id.String()))))
}

// makeResourceName builds the Kubernetes object name from the resource ID and a hex hash of the ID.
//
// The kubernetes resource names we use are built according to the following format:
//
//	resource.<resource name>.<id hash> (for a resource)
//	scope.<resource name>.<id hash> (for a scope)
func makeResourceName(id resources.ID, hash string) string {
	// The hash is truncated to 40 hex characters (the width of the legacy SHA-1 hash) so the name
	// continues to fit within the 253-character Kubernetes object name limit.
	const hashLength = 40
	if len(hash) > hashLength {
		hash = hash[:hashLength]
	}

	prefix := database.UCPResourcePrefix
	if id.IsScope() {
		prefix = database.UCPScopePrefix
	}

	normalizedName := normalizeName(id.Name())
	// 211 = 253 (max length of Kubernetes Object name) - 40 (hex hash length) - 2 (dot separators)
	maxResourceNameLen := 211 - len(prefix)
	if len(normalizedName) >= maxResourceNameLen {
		normalizedName = normalizedName[:maxResourceNameLen]
	}

	// example: resource.resource1.ec291e26078b7ea8a74abfac82530005a0ecbf15
	return fmt.Sprintf("%s.%s.%s", prefix, normalizedName, hash)
}

// getResourceWithFallback reads the Kubernetes Resource object that stores id, preferring the
// current (SHA-256) entry and falling back to the legacy (SHA-1) entry written by older versions
// of Radius. If neither object contains id, it retains the current object when present, so a new
// entry can share its collision bucket. The returned error is NotFound only if neither object exists. See
// https://github.com/radius-project/radius/issues/8084.
func (c *APIServerClient) getResourceWithFallback(ctx context.Context, id resources.ID, resource *ucpv1alpha1.Resource) error {
	currentKey := runtimeclient.ObjectKey{Namespace: c.namespace, Name: resourceName(id)}
	currentErr := c.get(ctx, currentKey, resource)
	if currentErr != nil && !apierrors.IsNotFound(currentErr) {
		return currentErr
	}
	if currentErr == nil && findIndex(resource, id) != nil {
		return nil
	}

	legacy := ucpv1alpha1.Resource{}
	legacyErr := c.get(ctx, runtimeclient.ObjectKey{Namespace: c.namespace, Name: legacyResourceName(id)}, &legacy)
	if legacyErr != nil && !apierrors.IsNotFound(legacyErr) {
		return legacyErr
	}
	if legacyErr == nil && findIndex(&legacy, id) != nil {
		*resource = legacy
		return nil
	}

	// The migration may have moved the entry from the legacy object to the current object between the two reads. It
	// writes the current copy before it removes the legacy copy, so one more read of the current object finds it.
	recheck := ucpv1alpha1.Resource{}
	recheckErr := c.get(ctx, currentKey, &recheck)
	if recheckErr != nil && !apierrors.IsNotFound(recheckErr) {
		return recheckErr
	}
	if recheckErr == nil {
		*resource = recheck
		return nil
	}
	if legacyErr == nil {
		*resource = legacy
	}

	return legacyErr
}

// getResourceForSave reads the Kubernetes Resource object that should store id for a write. It
// prefers the current (SHA-256) name, but falls back to the legacy (SHA-1) name when id's entry
// already exists there so existing data is updated in place rather than duplicated. New resources
// are created under the current name. It returns the name to operate under, the object (empty if it
// does not exist), and whether the object already exists. See
// https://github.com/radius-project/radius/issues/8084.
func (c *APIServerClient) getResourceForSave(ctx context.Context, id resources.ID) (string, ucpv1alpha1.Resource, bool, error) {
	current := resourceName(id)
	resource := ucpv1alpha1.Resource{}
	err := c.getResourceWithFallback(ctx, id, &resource)
	if err != nil && !apierrors.IsNotFound(err) {
		return "", ucpv1alpha1.Resource{}, false, err
	}

	if err == nil && (resource.Name == current || findIndex(&resource, id) != nil) {
		return resource.Name, resource, true, nil
	}

	return current, ucpv1alpha1.Resource{}, false, nil
}

func assignLabels(resource *ucpv1alpha1.Resource) labels.Set {
	set := labels.Set{}
	for _, entry := range resource.Entries {
		// It's ok to ignore errors here because we've already validated this data. We don't expect this to happen
		// unless someone manually tampers with our data.
		id, err := resources.Parse(entry.ID)
		if err != nil {
			continue
		}

		prefix, rootScope, _, resourceType := databaseutil.ExtractStorageParts(id)
		set[LabelKind] = prefix

		// We need to take apart the root scope so we can turn it into key-value-pairs.
		parsedRootScope, err := resources.Parse(rootScope)
		if err != nil {
			continue
		}
		for _, scope := range parsedRootScope.ScopeSegments() {
			key := fmt.Sprintf(LabelScopeFormat, strings.ToLower(strings.ReplaceAll(scope.Type, resources.SegmentSeparator, "-")))
			value := strings.ToLower(scope.Name)

			existing, ok := set[key]
			if ok && existing != value {
				value = LabelValueMultiple
			}

			set[key] = value
		}

		// '/' is not valid in a label values, so we use '_'
		value := strings.ToLower(strings.ReplaceAll(resourceType, resources.SegmentSeparator, "_"))
		existing, ok := set[LabelResourceType]
		if ok && existing != value {
			value = LabelValueMultiple
		}

		set[LabelResourceType] = value
	}

	return set
}

func createLabelSelector(query database.Query) (labels.Selector, error) {
	id, err := resources.Parse(query.RootScope)
	if err != nil {
		return nil, err
	}

	selector := labels.NewSelector()
	if query.IsScopeQuery {
		requirement, err := labels.NewRequirement(LabelKind, selection.Equals, []string{databaseutil.ScopePrefix})
		if err != nil {
			return nil, err
		}

		selector = selector.Add(*requirement)
	} else {
		requirement, err := labels.NewRequirement(LabelKind, selection.Equals, []string{databaseutil.ResourcePrefix})
		if err != nil {
			return nil, err
		}

		selector = selector.Add(*requirement)
	}

	for _, scope := range id.ScopeSegments() {
		key := fmt.Sprintf(LabelScopeFormat, strings.ToLower(strings.ReplaceAll(scope.Type, resources.SegmentSeparator, "-")))
		value := strings.ToLower(scope.Name)

		requirement, err := labels.NewRequirement(key, selection.In, []string{value, LabelValueMultiple})
		if err != nil {
			return nil, err
		}

		selector = selector.Add(*requirement)
	}

	if query.ResourceType != "" {
		value := strings.ToLower(strings.ReplaceAll(query.ResourceType, resources.SegmentSeparator, "_"))
		requirement, err := labels.NewRequirement(LabelResourceType, selection.In, []string{value, LabelValueMultiple})
		if err != nil {
			return nil, err
		}

		selector = selector.Add(*requirement)
	}

	return selector, nil
}

func findIndex(resource *ucpv1alpha1.Resource, id resources.ID) *int {
	for i, entry := range resource.Entries {
		if strings.EqualFold(entry.ID, id.String()) {
			index := i
			return &index
		}
	}

	return nil
}

func readEntry(entry *ucpv1alpha1.ResourceEntry) (*database.Object, error) {
	var data any
	err := json.Unmarshal(entry.Data.Raw, &data)
	if err != nil {
		return nil, err
	}

	obj := database.Object{
		ID:   entry.ID,
		ETag: entry.ETag,
		Data: data,
	}

	return &obj, nil
}

func read(resource *ucpv1alpha1.Resource, id resources.ID) (*database.Object, error) {
	for _, entry := range resource.Entries {
		if strings.EqualFold(entry.ID, id.String()) {
			return readEntry(&entry)
		}
	}

	return nil, nil
}

func convert(obj *database.Object) (*ucpv1alpha1.ResourceEntry, error) {
	raw, err := json.Marshal(obj.Data)
	if err != nil {
		return nil, err
	}

	resource := ucpv1alpha1.ResourceEntry{
		ID:   obj.ID,
		ETag: etag.New(raw), // Don't trust the ETag on the object, it's likely unset.
		Data: &runtime.RawExtension{Raw: raw},
	}

	return &resource, nil
}
