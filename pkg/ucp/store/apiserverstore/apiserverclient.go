// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

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
// Each Kubernetes Resource object stores a list of UCP resources. Since we use SHA1 to generate hashes,
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
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/go-logr/logr"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/store"
	ucpv1alpha1 "github.com/project-radius/radius/pkg/ucp/store/apiserverstore/api/ucp.dev/v1alpha1"
	"github.com/project-radius/radius/pkg/ucp/store/storeutil"
	"github.com/project-radius/radius/pkg/ucp/util/etag"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
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

	// RetryCount is the number of retries we will make on optimisitic concurrency failures. The need for retries is **rare** because
	// it only happens on concurrent operations to the same UCP resource or on a hash collision.
	RetryCount = 10
)

func NewAPIServerClient(client runtimeclient.Client, namespace string) *APIServerClient {
	return &APIServerClient{client: client, namespace: namespace}
}

var _ store.StorageClient = (*APIServerClient)(nil)

type APIServerClient struct {
	client    runtimeclient.Client
	namespace string

	// readyChan is used for testing concurrency behavior. This will be signaled when the client is ready to perform a write.
	readyChan chan<- struct{}

	// waitChan is used for testing concurrency behavior. This will be read from before the client performs a write.
	waitChan <-chan struct{}
}

func (c *APIServerClient) Query(ctx context.Context, query store.Query, options ...store.QueryOptions) (*store.ObjectQueryResult, error) {
	if ctx == nil {
		return nil, &store.ErrInvalid{Message: "invalid argument. 'ctx' is required"}
	}
	if query.RootScope == "" {
		return nil, &store.ErrInvalid{Message: "invalid argument. 'query.RootScope' is required"}
	}
	if query.IsScopeQuery && query.RoutingScopePrefix != "" {
		return nil, &store.ErrInvalid{Message: "invalid argument. 'query.RoutingScopePrefix' is not supported for scope queries"}
	}

	selector, err := createLabelSelector(query)
	if err != nil {
		return nil, err
	}

	rs := ucpv1alpha1.ResourceList{}
	err = c.client.List(ctx, &rs, runtimeclient.InNamespace(c.namespace), runtimeclient.MatchingLabelsSelector{Selector: selector})
	if err != nil {
		return nil, err
	}

	results := store.ObjectQueryResult{}
	for _, resource := range rs.Items {
		for _, entry := range resource.Entries {
			id, err := resources.Parse(entry.ID)
			if err != nil {
				// Ignore invalid IDs when querying, we don't want a single piece of bad data to
				// break all queries.
				logger := logr.FromContextOrDiscard(ctx)
				logger.Error(err, "found an invalid resource id as part of a query", "name", resource.Name, "namespace", resource.Namespace)
				continue
			}

			if storeutil.IDMatchesQuery(id, query) {
				converted, err := readEntry(&entry)
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
		}
	}

	return &results, nil
}

func (c *APIServerClient) Get(ctx context.Context, id string, options ...store.GetOptions) (*store.Object, error) {
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

	resourceName := resourceName(parsed)

	resource := ucpv1alpha1.Resource{}
	err = c.client.Get(ctx, runtimeclient.ObjectKey{Namespace: c.namespace, Name: resourceName}, &resource)
	if err != nil && apierrors.IsNotFound(err) {
		return nil, &store.ErrNotFound{}
	} else if err != nil {
		return nil, err
	}

	obj, err := read(&resource, parsed)
	if err != nil {
		return nil, err
	} else if obj == nil {
		return nil, &store.ErrNotFound{}
	}

	return obj, nil
}

func (c *APIServerClient) Delete(ctx context.Context, id string, options ...store.DeleteOptions) error {
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

	resourceName := resourceName(parsed)

	config := store.NewDeleteConfig(options...)

	err = c.doWithRetry(ctx, func() (bool, error) {
		resource := ucpv1alpha1.Resource{}
		err := c.client.Get(ctx, runtimeclient.ObjectKey{Namespace: c.namespace, Name: resourceName}, &resource)
		if err != nil && apierrors.IsNotFound(err) && config.ETag != "" {
			return false, &store.ErrConcurrency{}
		} else if err != nil && apierrors.IsNotFound(err) {
			return false, &store.ErrNotFound{}
		} else if err != nil {
			return false, err
		}

		index := findIndex(&resource, parsed)
		if index == nil {
			return false, &store.ErrNotFound{}
		}

		if config.ETag != "" && config.ETag != resource.Entries[*index].ETag {
			return false, &store.ErrConcurrency{}
		}

		c.synchronize()

		if len(resource.Entries) == 1 {
			// If this is the last resource we can delete (common case)
			options := runtimeclient.DeleteOptions{
				Preconditions: &v1.Preconditions{
					UID:             &resource.UID,
					ResourceVersion: &resource.ResourceVersion,
				},
			}
			err := c.client.Delete(ctx, &resource, &options)
			if err != nil && apierrors.IsNotFound(err) {
				return false, &store.ErrNotFound{}
			} else if apierrors.IsConflict(err) {
				return true, err // RETRY this!
			} else if err != nil {
				return false, err
			}
		} else {
			// If there was more than one resource we need to update. There's no need to explicitly
			// pass the options here as OCC is implicit.
			resource.Entries = append(resource.Entries[:*index], resource.Entries[*index+1:]...)
			resource.Labels = assignLabels(&resource)

			err := c.client.Update(ctx, &resource)
			if err != nil && apierrors.IsNotFound(err) {
				return false, &store.ErrNotFound{}
			} else if apierrors.IsConflict(err) {
				return true, err // RETRY this!
			} else if err != nil {
				return false, err
			}
		}

		return false, nil
	})

	return err
}

func (c *APIServerClient) Save(ctx context.Context, obj *store.Object, options ...store.SaveOptions) error {
	if ctx == nil {
		return &store.ErrInvalid{Message: "invalid argument. 'ctx' is required"}
	}
	if obj == nil {
		return &store.ErrInvalid{Message: "invalid argument. 'obj' is required"}
	}

	id, err := resources.Parse(obj.ID)
	if err != nil {
		return err
	}

	resourceName := resourceName(id)

	config := store.NewSaveConfig(options...)

	err = c.doWithRetry(ctx, func() (bool, error) {
		found := true
		resource := ucpv1alpha1.Resource{}
		err = c.client.Get(ctx, runtimeclient.ObjectKey{Namespace: c.namespace, Name: resourceName}, &resource)
		if err != nil && apierrors.IsNotFound(err) {
			found = false
		} else if err != nil {
			return false, err
		}

		// These need to be initialized if we're creating the object.
		resource.Name = resourceName
		resource.Namespace = c.namespace

		converted, err := convert(obj)
		if err != nil {
			return false, err
		}

		// Set the ETag so the caller can see the computed value.
		obj.ETag = converted.ETag

		index := findIndex(&resource, id)
		if index == nil && config.ETag != "" {
			// The ETag is only meaning for a replace/update operation not a create. We treat
			// the absence of the resource as a match failure.
			return false, &store.ErrConcurrency{}
		} else if index == nil {
			resource.Entries = append(resource.Entries, *converted)
		} else {
			if config.ETag != "" && config.ETag != resource.Entries[*index].ETag {
				return false, &store.ErrConcurrency{}
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

	return err
}

func (c *APIServerClient) doWithRetry(ctx context.Context, action func() (bool, error)) error {
	for i := 0; i < RetryCount; i++ {
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
	return &store.ErrConcurrency{}
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
	// The kubernetes resource names we use are built according to the following format
	//
	// resource.<resource name>.<id hash> (for a resource)
	// scope.<resource name>.<id hash> (for a scope)
	hasher := sha1.New()
	_, _ = hasher.Write([]byte(strings.ToLower(id.String())))
	hash := hasher.Sum(nil)

	prefix := store.UCPResourcePrefix
	if id.IsScope() {
		prefix = store.UCPScopePrefix
	}

	noramlizedName := normalizeName(id.Name())
	// 211 = 253 (max length of Kubernetes Object name) - 40 (hex hash length) - 2 (dot separators)
	maxResourceNameLen := 211 - len(prefix)
	if len(noramlizedName) >= maxResourceNameLen {
		noramlizedName = noramlizedName[:maxResourceNameLen]
	}

	// example: resource.resource1.ec291e26078b7ea8a74abfac82530005a0ecbf15
	return fmt.Sprintf("%s.%s.%x", prefix, noramlizedName, hash)
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

		prefix, rootScope, _, resourceType := storeutil.ExtractStorageParts(id)
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

func createLabelSelector(query store.Query) (labels.Selector, error) {
	id, err := resources.Parse(query.RootScope)
	if err != nil {
		return nil, err
	}

	selector := labels.NewSelector()
	if query.IsScopeQuery {
		requirement, err := labels.NewRequirement(LabelKind, selection.Equals, []string{storeutil.ScopePrefix})
		if err != nil {
			return nil, err
		}

		selector = selector.Add(*requirement)
	} else {
		requirement, err := labels.NewRequirement(LabelKind, selection.Equals, []string{storeutil.ResourcePrefix})
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

func readEntry(entry *ucpv1alpha1.ResourceEntry) (*store.Object, error) {
	var data any
	err := json.Unmarshal(entry.Data.Raw, &data)
	if err != nil {
		return nil, err
	}

	obj := store.Object{
		Metadata: store.Metadata{
			ID:          entry.ID,
			ETag:        entry.ETag,
			APIVersion:  entry.APIVersion,
			ContentType: entry.ContentType,
		},
		Data: data,
	}

	return &obj, nil
}

func read(resource *ucpv1alpha1.Resource, id resources.ID) (*store.Object, error) {
	for _, entry := range resource.Entries {
		if strings.EqualFold(entry.ID, id.String()) {
			return readEntry(&entry)
		}
	}

	return nil, nil
}

func convert(obj *store.Object) (*ucpv1alpha1.ResourceEntry, error) {
	raw, err := json.Marshal(obj.Data)
	if err != nil {
		return nil, err
	}

	resource := ucpv1alpha1.ResourceEntry{
		ID:          obj.ID,
		APIVersion:  obj.APIVersion,
		ETag:        etag.New(raw), // Don't trust the ETag on the object, it's likely unset.
		ContentType: obj.ContentType,
		Data:        &runtime.RawExtension{Raw: raw},
	}

	return &resource, nil
}
