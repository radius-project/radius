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
	"encoding/json"
	"errors"
	"fmt"
	"strings"

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

// annotationMigration holds the migration records of the entries of a current object: a JSON object that maps the
// lowercase ID of an entry to its migrationRecord. Only entries that had a legacy copy have a record, so an installation
// that never stored a legacy entry has none. The records stay until the release that removes the legacy names.
const annotationMigration = "ucp.dev/sha256-migration"

const (
	// recordLegacy means a save wrote the entry to its legacy object while the current object held no copy of it.
	recordLegacy = "legacy"

	// recordMigrated means the migration moved the entry from its legacy object, or found an identical current copy.
	recordMigrated = "migrated"

	// recordDeleted is a tombstone: the entry had a legacy copy and was deleted since. A delete writes it before it
	// removes any copy, so a migrator that read the legacy copy before finds the tombstone, or fails to write because
	// the tombstone changed the current object, and never publishes the deleted value again. Nothing replaces a
	// tombstone, so a delete that finds one does not write it again.
	recordDeleted = "deleted"
)

// migrationRecord is the record of one entry in annotationMigration.
type migrationRecord struct {
	// State is recordLegacy, recordMigrated, or recordDeleted.
	State string `json:"state"`

	// ETag is the ETag of the value the current object last received as a copy of the legacy value, for
	// recordMigrated. A save to the current object clears it, so a current value with this ETag is a copy that nothing
	// wrote since.
	ETag string `json:"etag,omitempty"`

	// Fence counts the saves to the legacy object that changed the current object, so that each of them changes it. See
	// fenceLegacyWrite and refreshCopy.
	Fence int `json:"fence,omitempty"`
}

// isCopy reports whether record marks entry, the current copy of an entry, as a copy of a legacy value that nothing
// saved to the current object since.
func isCopy(record migrationRecord, entry ucpv1alpha1.ResourceEntry) bool {
	return record.State == recordMigrated && record.ETag != "" && record.ETag == entry.ETag
}

// migrationRecordOf returns the migration record of id in resource, and whether it has one.
func migrationRecordOf(resource *ucpv1alpha1.Resource, id resources.ID) (migrationRecord, bool, error) {
	records, err := migrationRecords(resource)
	if err != nil {
		return migrationRecord{}, false, err
	}

	record, ok := records[strings.ToLower(id.String())]
	return record, ok, nil
}

// setMigrationRecord sets the migration record of id in resource.
func setMigrationRecord(resource *ucpv1alpha1.Resource, id resources.ID, record migrationRecord) error {
	records, err := migrationRecords(resource)
	if err != nil {
		return err
	}

	records[strings.ToLower(id.String())] = record
	value, err := json.Marshal(records)
	if err != nil {
		return err
	}

	if resource.Annotations == nil {
		resource.Annotations = map[string]string{}
	}
	resource.Annotations[annotationMigration] = string(value)
	return nil
}

func migrationRecords(resource *ucpv1alpha1.Resource) (map[string]migrationRecord, error) {
	records := map[string]migrationRecord{}
	value, ok := resource.Annotations[annotationMigration]
	if !ok {
		return records, nil
	}

	if err := json.Unmarshal([]byte(value), &records); err != nil {
		return nil, fmt.Errorf("failed to read the %s annotation of %s: %w", annotationMigration, resource.Name, err)
	}

	return records, nil
}

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
	// entryWrittenAfterDelete means the current object holds a tombstone of the entry, so the legacy copy was written
	// after the entry was deleted. It was left in place.
	entryWrittenAfterDelete
	// entryGone means the legacy copy no longer exists, for example because another replica migrated it.
	entryGone
	// entryBusy means concurrent writes prevented progress. The entry is retried on a later pass.
	entryBusy
)

// errGateClosed stops a pass that must wait for the gate.
var errGateClosed = errors.New("the gate is closed")

// MigrateLegacyNames moves entries stored under legacy (SHA-1) Kubernetes object names to their current (SHA-256)
// object names. Each entry keeps its data and ETag, so callers holding an ETag can continue to use it.
//
// gate decides when entries may be moved. Releases before this one read and write an entry under its legacy name
// until it has a current copy, without making their writes visible in a copy, so the pass stops while one of their
// pods runs. The gate is checked again before each entry.
//
// The move copies the entry to the current object before it removes the legacy copy, so reads, which prefer the
// current object and fall back to the legacy object, always find it. The current object records the move in
// annotationMigration. Every attempt reads the current object before the legacy object, and every write is conditional
// on the version of the object it read. Every other writer that changes the legacy copy also changes the current
// object, so a migrator does not keep a copy of a value that the legacy object no longer holds:
//
//   - A delete of an entry that had a legacy copy writes a tombstone in the current object before it removes any copy,
//     and a migrator never copies an entry over a tombstone.
//   - A save that wrote the legacy object then changes the current object. If the current object holds no copy, the
//     change fails the write of a migrator that read the replaced value (see fenceLegacyWrite). If a migrator wrote its
//     copy first, the save replaces the copy with the value the legacy object holds (see refreshCopy).
//
// An entry that exists under both names with identical content is deduplicated. A legacy value that changed after
// the current object received a copy of it, which a save that read the legacy copy before the copy existed writes if
// it stops before it makes the write visible, replaces the copy, provided nothing saved the entry to the current object
// since. Other differing copies are left in place and reported as a conflict; reads already use the current copy.
//
// The method is idempotent and safe to run concurrently with normal client operations and with other replicas. See
// https://github.com/radius-project/radius/issues/8084.
func (c *APIServerClient) MigrateLegacyNames(ctx context.Context, gate hashmigration.Gate) (hashmigration.Result, error) {
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

			reason, err := hashmigration.Wait(ctx, gate)
			if err != nil {
				return err
			} else if reason != "" {
				result.Remaining++
				result.Message = reason
				return errGateClosed
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
			case entryWrittenAfterDelete:
				logger.Info("A stored entry has a legacy copy that was written after the entry was deleted; leaving it", "id", entry.ID)
				result.Conflicts++
			case entryBusy:
				result.Remaining++
			}
		}

		return nil
	})
	if errors.Is(err, errGateClosed) {
		err = nil
	}

	return result, err
}

// migrateEntry moves the entry for id from its legacy object to its current object.
func (c *APIServerClient) migrateEntry(ctx context.Context, id resources.ID) (entryOutcome, error) {
	legacyKey := runtimeclient.ObjectKey{Namespace: c.namespace, Name: legacyResourceName(id)}
	currentKey := runtimeclient.ObjectKey{Namespace: c.namespace, Name: resourceName(id)}

	// copied records that this call wrote the current copy, so the entry counts as moved rather than deduplicated.
	copied := false
	for range RetryCount {
		// The current object is read first. A legacy value read after it is at least as new as every value the
		// current object received as a copy of the legacy value.
		current := ucpv1alpha1.Resource{}
		err := c.get(ctx, currentKey, &current)
		currentExists := err == nil
		if err != nil && !apierrors.IsNotFound(err) {
			return 0, err
		}

		legacy := ucpv1alpha1.Resource{}
		err = c.get(ctx, legacyKey, &legacy)
		if apierrors.IsNotFound(err) {
			return entryGone, nil
		} else if err != nil {
			return 0, err
		}

		legacyIndex := findIndex(&legacy, id)
		if legacyIndex == nil {
			return entryGone, nil
		}
		legacyEntry := legacy.Entries[*legacyIndex]

		var currentIndex *int
		record, recorded := migrationRecord{}, false
		if currentExists {
			currentIndex = findIndex(&current, id)
			record, recorded, err = migrationRecordOf(&current, id)
			if err != nil {
				return 0, err
			}
		}

		switch {
		case currentIndex != nil && sameEntry(current.Entries[*currentIndex], legacyEntry):
			// Only the legacy copy is left to remove. A record makes a later delete of the entry leave a tombstone. The
			// current object also changes before the legacy copy is removed if it holds a copy that nothing saved since:
			// a save that read an older legacy value to refresh that copy then fails to write it (see refreshCopy).
			next, write := record, true
			switch {
			case !recorded || record.State == recordLegacy:
				next = migrationRecord{State: recordMigrated, ETag: current.Entries[*currentIndex].ETag}
			case isCopy(record, current.Entries[*currentIndex]):
				next.Fence++
			default:
				write = false
			}

			if write {
				if err := setMigrationRecord(&current, id, next); err != nil {
					return 0, err
				}

				if err := c.client.Update(ctx, &current); isRetryable(err) || apierrors.IsNotFound(err) {
					continue
				} else if err != nil {
					return 0, err
				}
			}
		case currentIndex != nil && isCopy(record, current.Entries[*currentIndex]):
			// The current value is a copy that nothing saved since, and the legacy value changed after the copy was
			// taken, so the legacy value is newer.
			current.Entries[*currentIndex] = *legacyEntry.DeepCopy()
			current.Labels = assignLabels(&current)
			if err := setMigrationRecord(&current, id, migrationRecord{State: recordMigrated, ETag: legacyEntry.ETag}); err != nil {
				return 0, err
			}

			if err := c.client.Update(ctx, &current); isRetryable(err) || apierrors.IsNotFound(err) {
				continue
			} else if err != nil {
				return 0, err
			}
			copied = true
		case currentIndex != nil:
			return entryConflict, nil
		case record.State == recordDeleted:
			// A legacy copy was written after the entry was deleted, which only a release that knows nothing of current
			// names does, or a delete is removing it now.
			return entryWrittenAfterDelete, nil
		default:
			if !currentExists {
				current = ucpv1alpha1.Resource{ObjectMeta: v1.ObjectMeta{Name: currentKey.Name, Namespace: c.namespace}}
			}
			current.Entries = append(current.Entries, *legacyEntry.DeepCopy())
			current.Labels = assignLabels(&current)
			if err := setMigrationRecord(&current, id, migrationRecord{State: recordMigrated, ETag: legacyEntry.ETag}); err != nil {
				return 0, err
			}

			if currentExists {
				err = c.client.Update(ctx, &current)
			} else {
				err = c.client.Create(ctx, &current)
			}
			if isRetryable(err) || apierrors.IsNotFound(err) {
				continue
			} else if err != nil {
				return 0, err
			}
			copied = true
		}

		err = c.removeEntry(ctx, &legacy, *legacyIndex)
		if isRetryable(err) || apierrors.IsNotFound(err) {
			continue
		} else if err != nil {
			return 0, err
		}

		if copied {
			return entryMoved, nil
		}
		return entryDeduplicated, nil
	}

	return entryBusy, nil
}

// removeEntry removes the entry at index from resource, deleting the object when it holds no other entries and no
// migration records. Both operations fail with a conflict if resource changed after it was read.
func (c *APIServerClient) removeEntry(ctx context.Context, resource *ucpv1alpha1.Resource, index int) error {
	if _, recorded := resource.Annotations[annotationMigration]; len(resource.Entries) == 1 && !recorded {
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

// retireEntry removes the entry at index from the current object resource and records a tombstone of id in its
// place. The object is kept even if it holds no other entries. It fails with a conflict if resource changed after it
// was read.
func (c *APIServerClient) retireEntry(ctx context.Context, resource *ucpv1alpha1.Resource, index int, id resources.ID) error {
	if err := setMigrationRecord(resource, id, migrationRecord{State: recordDeleted}); err != nil {
		return err
	}

	entries := make([]ucpv1alpha1.ResourceEntry, 0, len(resource.Entries)-1)
	entries = append(entries, resource.Entries[:index]...)
	entries = append(entries, resource.Entries[index+1:]...)
	resource.Entries = entries
	resource.Labels = assignLabels(resource)
	return c.client.Update(ctx, resource)
}

// writeTombstone records a tombstone of id in current, the current object of id, before a delete removes the copies of
// an entry that had a legacy copy. It keeps the entries of current, and creates the object if current is nil. Both
// operations fail if the object changed after it was read, or was created since. It returns the object as written.
func (c *APIServerClient) writeTombstone(ctx context.Context, current *ucpv1alpha1.Resource, id resources.ID) (*ucpv1alpha1.Resource, error) {
	if current == nil {
		current = &ucpv1alpha1.Resource{ObjectMeta: v1.ObjectMeta{Name: resourceName(id), Namespace: c.namespace}}
		if err := setMigrationRecord(current, id, migrationRecord{State: recordDeleted}); err != nil {
			return nil, err
		}

		return current, c.client.Create(ctx, current)
	}

	if err := setMigrationRecord(current, id, migrationRecord{State: recordDeleted}); err != nil {
		return nil, err
	}

	return current, c.client.Update(ctx, current)
}

// fenceLegacyWrite changes current, the current object of id, after a save wrote id to its legacy object while current
// held no copy of it, and creates the object if it does not exist. A migrator that read the current object before, and
// the legacy object before the save wrote it, would copy the value the save replaced. Its write of the copy is
// conditional on the version of the current object it read, so it fails, and the migrator reads both objects again.
// The record counts the fences, so that every fence changes the object.
func (c *APIServerClient) fenceLegacyWrite(ctx context.Context, current *ucpv1alpha1.Resource, exists bool, id resources.ID) error {
	if !exists {
		*current = ucpv1alpha1.Resource{ObjectMeta: v1.ObjectMeta{Name: resourceName(id), Namespace: c.namespace}}
	}

	record, recorded, err := migrationRecordOf(current, id)
	if err != nil {
		return err
	} else if recorded && record.State == recordDeleted {
		// A migrator never copies an entry over a tombstone, so no fence is needed. Replacing the tombstone would let a
		// migrator copy the written value while a delete removes it.
		return nil
	}

	fence := 1
	if recorded && record.State == recordLegacy {
		fence = record.Fence + 1
	}
	if err := setMigrationRecord(current, id, migrationRecord{State: recordLegacy, Fence: fence}); err != nil {
		return err
	}

	if exists {
		return c.client.Update(ctx, current)
	}
	return c.client.Create(ctx, current)
}

// refreshCopy runs after a save wrote id to its legacy object, when current, the current object of id, holds at index
// a copy of a legacy value that nothing saved to the current object since. The copy may be older than the write, so it
// is replaced by the entry the legacy object holds now: the written value, or the value a later save wrote there. The
// object changes even if the copy is up to date, so a save that read an older legacy value, and has not yet written
// it to the copy, fails to write it and reads both objects again. It fails with a conflict if current changed after it
// was read.
func (c *APIServerClient) refreshCopy(ctx context.Context, current *ucpv1alpha1.Resource, index int, id resources.ID, record migrationRecord) error {
	legacy := ucpv1alpha1.Resource{}
	err := c.get(ctx, runtimeclient.ObjectKey{Namespace: c.namespace, Name: legacyResourceName(id)}, &legacy)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	record.Fence++
	if legacyIndex := findIndex(&legacy, id); err == nil && legacyIndex != nil && legacy.Entries[*legacyIndex].ETag != current.Entries[index].ETag {
		current.Entries[index] = *legacy.Entries[*legacyIndex].DeepCopy()
		current.Labels = assignLabels(current)
		record.ETag = legacy.Entries[*legacyIndex].ETag
	}
	if err := setMigrationRecord(current, id, record); err != nil {
		return err
	}

	return c.client.Update(ctx, current)
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
