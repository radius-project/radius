/*
Copyright 2026 The Radius Authors.

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

package apiserverstore

import (
	"bytes"
	"context"
	"time"
	"uuid"

	"github.com/radius-project/radius/pkg/components/database"
	ucpv1alpha1 "github.com/radius-project/radius/pkg/components/database/apiserverstore/api/ucp.dev/v1alpha1"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	"github.com/radius-project/radius/pkg/upgrade/hashmigration"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// listPageSize bounds the number of Kubernetes objects read per list call during maintenance scans.
const listPageSize = 250

// annotationDeduplicated is set to a new value on the current object of an entry each time a migrator removes a legacy
// copy of the entry that it did not copy itself. See legacyCopyRemoved.
const annotationDeduplicated = "ucp.dev/sha256-deduplicated"

var _ database.Enumerator = (*APIServerClient)(nil)

// Enumerate implements database.Enumerator. An entry that is stored under both its current and legacy object names is
// visited once for each copy.
func (c *APIServerClient) Enumerate(ctx context.Context, fn func(obj *database.Object) error) error {
	return c.forEachObject(ctx, func(resource *ucpv1alpha1.Resource) error {
		for i := range resource.Entries {
			obj, err := readEntry(&resource.Entries[i])
			if err != nil {
				return err
			}

			if err := fn(obj); err != nil {
				return err
			}
		}

		return nil
	})
}

// forEachObject calls fn for every Kubernetes object that stores entries, reading them in pages.
func (c *APIServerClient) forEachObject(ctx context.Context, fn func(resource *ucpv1alpha1.Resource) error) error {
	options := &runtimeclient.ListOptions{Namespace: c.namespace, Limit: listPageSize}
	for {
		list := ucpv1alpha1.ResourceList{}
		if err := c.list(ctx, &list, options); err != nil {
			return err
		}

		for i := range list.Items {
			if err := fn(&list.Items[i]); err != nil {
				return err
			}
		}

		if list.Continue == "" {
			return nil
		}

		options.Continue = list.Continue
	}
}

// entryOutcome is the result of migrating one entry.
type entryOutcome int

const (
	// entryMoved means the entry now exists only under its current object name.
	entryMoved entryOutcome = iota
	// entryDeduplicated means an identical current copy already existed and the legacy copy was removed.
	entryDeduplicated
	// entryConflict means the entry exists under both names with different content and was left in place.
	entryConflict
	// entryGone means the legacy copy no longer exists, for example because another replica migrated it.
	entryGone
	// entryBusy means concurrent writes prevented progress. The entry is retried on a later pass.
	entryBusy
)

// MigrateLegacyNames moves entries stored under legacy (SHA-1) Kubernetes object names to their current (SHA-256)
// object names. Each entry keeps its data and ETag, so callers holding an ETag can continue to use it.
//
// The move copies the entry to the current object before removing it from the legacy object, using Kubernetes
// optimistic concurrency for every write. A reader therefore always finds the entry: reads prefer the current object
// and fall back to the legacy object. An entry that exists under both names with identical content is deduplicated. If
// the copies differ, both are left in place and reported as a conflict; reads already use the current copy.
//
// A delete removes every copy of an entry. A delete that runs after the move read the legacy copy and before it wrote
// the current copy removes only the legacy copy, so the move then removes the current copy it wrote, unless the current
// object changed since. The move does that even if the pass is canceled after it started to write the copy. The delete
// also reads the current object again after it removed a legacy copy, and removes a copy of the value it removed, so a
// copy of a deleted entry remains only if the process is killed in the milliseconds between the write of the copy and
// its removal, after the delete read the current object again. A migrator that removes a legacy copy it did not copy
// itself first changes the current object, so a concurrent migrator never mistakes that removal for a delete and
// removes the copy.
//
// The method is idempotent and safe to run concurrently with normal client operations and with other replicas. See
// https://github.com/radius-project/radius/issues/8084.
func (c *APIServerClient) MigrateLegacyNames(ctx context.Context) (hashmigration.Result, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	result := hashmigration.Result{}
	err := c.forEachObject(ctx, func(resource *ucpv1alpha1.Resource) error {
		for _, entry := range resource.Entries {
			id, err := resources.Parse(entry.ID)
			if err != nil {
				// No lookup can reach an entry whose ID cannot be parsed, so there is nothing to migrate.
				result.Skipped++
				continue
			}

			if resource.Name == resourceName(id) {
				continue
			}

			if resource.Name != legacyResourceName(id) {
				logger.Info("Skipping a stored entry whose object name does not match its ID", "object", resource.Name, "id", entry.ID)
				result.Skipped++
				continue
			}

			outcome, err := c.migrateEntry(ctx, id)
			if err != nil {
				logger.Info("Unable to migrate a stored entry to its SHA-256 object name", "id", entry.ID, "error", err.Error())
				result.Remaining++
				result.Message = err.Error()
				continue
			}

			switch outcome {
			case entryMoved:
				result.Migrated++
			case entryDeduplicated:
				result.Deduplicated++
			case entryConflict:
				logger.Info("A stored entry has different copies under its legacy and current object names; leaving both", "id", entry.ID)
				result.Conflicts++
			case entryBusy:
				result.Remaining++
			}
		}

		return nil
	})

	return result, err
}

// moveFinishTimeout bounds a move, whose calls from the write of a copy on are not canceled with the pass.
const moveFinishTimeout = time.Minute

// migrateEntry moves the entry for id from its legacy object to its current object.
func (c *APIServerClient) migrateEntry(ctx context.Context, id resources.ID) (entryOutcome, error) {
	legacyKey := runtimeclient.ObjectKey{Namespace: c.namespace, Name: legacyResourceName(id)}
	currentKey := runtimeclient.ObjectKey{Namespace: c.namespace, Name: resourceName(id)}

	// copied is the legacy version this call wrote to the current object. It distinguishes a move performed here
	// from a pre-existing duplicate and lets the call promote a legacy update made after the copy. copiedVersion is the
	// resource version of the current object after that write.
	var copied *ucpv1alpha1.ResourceEntry
	copiedVersion := ""

	// From the write of a copy on, this call uses finishCtx, so it finishes the move, or removes the copy if a delete
	// removed the legacy copy, even if ctx is canceled. A shutdown in between would otherwise leave a copy of a deleted
	// entry.
	finishCtx, stopFinishing := context.WithTimeout(context.WithoutCancel(ctx), moveFinishTimeout)
	defer stopFinishing()
	finishing := false

	for range RetryCount {
		legacy := ucpv1alpha1.Resource{}
		err := c.get(ctx, legacyKey, &legacy)
		if apierrors.IsNotFound(err) {
			return c.legacyCopyRemoved(ctx, id, copied, copiedVersion)
		} else if err != nil {
			return 0, err
		}

		legacyIndex := findIndex(&legacy, id)
		if legacyIndex == nil {
			return c.legacyCopyRemoved(ctx, id, copied, copiedVersion)
		}
		legacyEntry := legacy.Entries[*legacyIndex]

		current := ucpv1alpha1.Resource{}
		err = c.get(ctx, currentKey, &current)
		currentExists := err == nil
		if err != nil && !apierrors.IsNotFound(err) {
			return 0, err
		}

		var currentIndex *int
		if currentExists {
			currentIndex = findIndex(&current, id)
		}

		switch {
		case currentIndex != nil && sameEntry(current.Entries[*currentIndex], legacyEntry):
			// The current copy matches; only the legacy copy needs to be removed. Unless this call wrote the current
			// object as it is now, another migrator may have just copied the entry and may remove its copy if the legacy
			// copy disappears while the current object is unchanged. Change the current object first, so it keeps it.
			if copied == nil || current.ResourceVersion != copiedVersion {
				if err := c.markDeduplicated(ctx, &current); isRetryable(err) || apierrors.IsNotFound(err) {
					continue
				} else if err != nil {
					return 0, err
				}
			}
		case currentIndex != nil && copied != nil && sameEntry(current.Entries[*currentIndex], *copied):
			// The legacy copy changed after this call copied it, and the current copy is unchanged. Only a writer that
			// knows nothing of current names updates a legacy copy once a current copy exists, so keep its newer value.
			current.Entries[*currentIndex] = *legacyEntry.DeepCopy()
			current.Labels = assignLabels(&current)
			if err := c.client.Update(ctx, &current); isRetryable(err) {
				continue
			} else if err != nil {
				return 0, err
			}
			copied = legacyEntry.DeepCopy()
			copiedVersion = current.ResourceVersion
		case currentIndex != nil:
			return entryConflict, nil
		default:
			if !finishing {
				if err := ctx.Err(); err != nil {
					return 0, err
				}
				ctx, finishing = finishCtx, true
			}

			entry := *legacyEntry.DeepCopy()
			if currentExists {
				current.Entries = append(current.Entries, entry)
				current.Labels = assignLabels(&current)
				err = c.client.Update(ctx, &current)
			} else {
				current = ucpv1alpha1.Resource{
					ObjectMeta: v1.ObjectMeta{Name: currentKey.Name, Namespace: c.namespace},
					Entries:    []ucpv1alpha1.ResourceEntry{entry},
				}
				current.Labels = assignLabels(&current)
				err = c.client.Create(ctx, &current)
			}

			if isRetryable(err) {
				continue
			} else if err != nil {
				return 0, err
			}
			copied = &entry
			copiedVersion = current.ResourceVersion
		}

		err = c.removeEntry(ctx, &legacy, *legacyIndex)
		if isRetryable(err) || apierrors.IsNotFound(err) {
			continue
		} else if err != nil {
			return 0, err
		}

		if copied != nil {
			return entryMoved, nil
		}
		return entryDeduplicated, nil
	}

	return entryBusy, nil
}

// removeEntry removes the entry at index from resource, deleting the object when it holds no other entries. Both
// operations fail with a conflict if resource changed after it was read.
func (c *APIServerClient) removeEntry(ctx context.Context, resource *ucpv1alpha1.Resource, index int) error {
	if len(resource.Entries) == 1 {
		return c.client.Delete(ctx, resource, &runtimeclient.DeleteOptions{
			Preconditions: &v1.Preconditions{UID: &resource.UID, ResourceVersion: &resource.ResourceVersion},
		})
	}

	entries := make([]ucpv1alpha1.ResourceEntry, 0, len(resource.Entries)-1)
	entries = append(entries, resource.Entries[:index]...)
	entries = append(entries, resource.Entries[index+1:]...)
	resource.Entries = entries
	resource.Labels = assignLabels(resource)
	return c.client.Update(ctx, resource)
}

// markDeduplicated changes the current object, with a resource-version precondition, before a legacy copy that this
// call did not copy is removed. See legacyCopyRemoved.
func (c *APIServerClient) markDeduplicated(ctx context.Context, current *ucpv1alpha1.Resource) error {
	if current.Annotations == nil {
		current.Annotations = map[string]string{}
	}
	current.Annotations[annotationDeduplicated] = uuid.New().String()
	return c.client.Update(ctx, current)
}

// legacyCopyRemoved handles a legacy copy that no longer exists. copied is the entry this call wrote to the current
// object, if any, and copiedVersion the resource version of the current object after that write.
//
// A delete removes every copy of an entry, so one that ran after this call wrote copied also removed copied, which
// changed the current object. A migrator that removes a legacy copy it did not copy changes the current object first
// (see markDeduplicated). If the current object is still as this call wrote it, the legacy copy was therefore removed
// by a delete that ran before copied existed, or by an older writer that knows only legacy names, and copied would
// bring the deleted resource back. It is removed, with a resource-version precondition.
func (c *APIServerClient) legacyCopyRemoved(ctx context.Context, id resources.ID, copied *ucpv1alpha1.ResourceEntry, copiedVersion string) (entryOutcome, error) {
	if copied == nil {
		return entryGone, nil
	}

	currentKey := runtimeclient.ObjectKey{Namespace: c.namespace, Name: resourceName(id)}
	for range RetryCount {
		current := ucpv1alpha1.Resource{}
		err := c.get(ctx, currentKey, &current)
		if apierrors.IsNotFound(err) {
			return entryGone, nil
		} else if err != nil {
			return 0, err
		}

		index := findIndex(&current, id)
		if index == nil {
			// A delete removed the copy too.
			return entryGone, nil
		} else if current.ResourceVersion != copiedVersion {
			// The entry was written again, or another migrator removed the legacy copy after this call copied it.
			return entryMoved, nil
		}

		err = c.removeEntry(ctx, &current, *index)
		if isRetryable(err) || apierrors.IsNotFound(err) {
			continue
		} else if err != nil {
			return 0, err
		}

		ucplog.FromContextOrDiscard(ctx).Info("A stored entry was deleted while it was moved to its SHA-256 object name; removed the copy the move wrote", "id", id.String())
		return entryGone, nil
	}

	return entryBusy, nil
}

// sameEntry reports whether two copies of an entry hold the same value. Identical data written by different versions
// can carry ETags in different formats, so the stored data is compared as well.
func sameEntry(a ucpv1alpha1.ResourceEntry, b ucpv1alpha1.ResourceEntry) bool {
	if a.ETag == b.ETag {
		return true
	}

	return a.Data != nil && b.Data != nil && bytes.Equal(a.Data.Raw, b.Data.Raw)
}

// isRetryable reports whether err is an optimistic-concurrency failure that a fresh read can resolve.
func isRetryable(err error) bool {
	return apierrors.IsConflict(err) || apierrors.IsAlreadyExists(err)
}
