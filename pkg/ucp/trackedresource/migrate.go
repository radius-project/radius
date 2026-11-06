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

package trackedresource

import (
	"context"
	"errors"
	"strings"
	"uuid"

	"github.com/go-logr/logr"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	"github.com/radius-project/radius/pkg/upgrade/hashmigration"
)

// radiusPlanePrefix is the prefix of the resource groups that hold tracked resource entries.
const radiusPlanePrefix = "/planes/radius/"

// MigrateLegacyEntries moves tracked resource entries stored under legacy (SHA-1) tracking IDs to their current
// (SHA-256) tracking IDs.
//
// Only idle entries are moved: an entry with an operation in flight is retried on a later pass. The move saves the
// current entry before deleting the legacy entry with its ETag, so a concurrent update makes the delete fail rather
// than lose the update. If the legacy entry changed or was deleted after the move read it, the move deletes the copy it
// saved, unless the copy changed since: the legacy entry remains the only entry until a later pass moves it, and the
// copy of a deleted resource's entry does not bring the entry back. A migrator that deletes a legacy entry it did not
// copy changes the current entry first, so a concurrent migrator never mistakes that delete for one of the updater and
// deletes its copy. See https://github.com/radius-project/radius/issues/8084.
func MigrateLegacyEntries(ctx context.Context, client database.Client) (hashmigration.Result, error) {
	result := hashmigration.Result{}
	groups, err := client.Query(ctx, database.Query{RootScope: "/planes", ScopeRecursive: true, IsScopeQuery: true, ResourceType: "resourcegroups"})
	if err != nil {
		return result, err
	}

	for _, group := range groups.Items {
		if !strings.HasPrefix(strings.ToLower(group.ID), radiusPlanePrefix) {
			continue
		}

		entries, err := client.Query(ctx, database.Query{RootScope: group.ID, ResourceType: datamodel.GenericResourceType})
		if err != nil {
			return result, err
		}

		for _, obj := range entries.Items {
			entryResult := migrateLegacyEntry(ctx, client, obj)
			result.Add(entryResult)
			if entryResult.Message != "" {
				result.Message = entryResult.Message
			}
		}
	}

	return result, nil
}

func migrateLegacyEntry(ctx context.Context, client database.Client, obj database.Object) hashmigration.Result {
	logger := ucplog.FromContextOrDiscard(ctx).WithValues("trackingID", obj.ID)
	entry := datamodel.GenericResource{}
	if err := obj.As(&entry); err != nil {
		logger.Info("Skipping a tracked resource entry that cannot be read", "error", err.Error())
		return hashmigration.Result{Skipped: 1}
	}

	original, err := resources.Parse(entry.Properties.ID)
	if err != nil {
		logger.Info("Skipping a tracked resource entry without a valid resource ID", "resourceID", entry.Properties.ID)
		return hashmigration.Result{Skipped: 1}
	}

	currentID := IDFor(original)
	if strings.EqualFold(obj.ID, currentID.String()) {
		return hashmigration.Result{}
	}

	if !strings.EqualFold(obj.ID, LegacyIDFor(original).String()) {
		logger.Info("Skipping a tracked resource entry whose ID does not match its resource", "resourceID", original.String())
		return hashmigration.Result{Skipped: 1}
	}

	if state := entry.AsyncProvisioningState; state != "" && !state.IsTerminal() {
		// An operation is in flight. It resolves the current ID first, so the entry is moved once it is idle.
		return hashmigration.Result{Remaining: 1}
	}

	current, err := client.Get(ctx, currentID.String())
	if errors.Is(err, &database.ErrNotFound{}) {
		return moveLegacyEntry(ctx, client, logger, obj, entry, currentID)
	} else if err != nil {
		return failed(logger, err)
	}

	// Both entries exist, and readers use the current entry, so the legacy entry is redundant. Another migrator may
	// have just saved the current entry as its copy, and would delete that copy if the legacy entry disappeared while the
	// copy is unchanged. Change the current entry first, so such a migrator keeps it.
	err = changeSalt(ctx, client, current)
	if errors.Is(err, &database.ErrConcurrency{}) || errors.Is(err, &database.ErrNotFound{}) {
		// The current entry changed or was deleted after it was read. The next pass re-evaluates the legacy entry.
		return hashmigration.Result{Remaining: 1}
	} else if err != nil {
		return failed(logger, err)
	}

	err = client.Delete(ctx, obj.ID, database.WithETag(obj.ETag))
	switch {
	case errors.Is(err, &database.ErrConcurrency{}), errors.Is(err, &database.ErrNotFound{}):
		// The legacy entry changed or was deleted after it was read. The next pass re-evaluates it.
		return hashmigration.Result{Remaining: 1}
	case err != nil:
		return failed(logger, err)
	default:
		return hashmigration.Result{Deduplicated: 1}
	}
}

// moveLegacyEntry saves the legacy entry obj under currentID, then deletes the legacy entry with its ETag.
func moveLegacyEntry(ctx context.Context, client database.Client, logger logr.Logger, obj database.Object, entry datamodel.GenericResource, currentID resources.ID) hashmigration.Result {
	moved := entry
	moved.ID = currentID.String()
	moved.Name = currentID.Name()
	copied := &database.Object{ID: currentID.String(), Data: &moved}
	err := client.Save(ctx, copied, database.WithCreateOnly())
	if errors.Is(err, &database.ErrConcurrency{}) {
		// Another writer created the current entry after it was read. Keep it; the next pass removes the legacy entry.
		return hashmigration.Result{Remaining: 1}
	} else if err != nil {
		return failed(logger, err)
	}

	err = client.Delete(ctx, obj.ID, database.WithETag(obj.ETag))
	switch {
	case err == nil:
		return hashmigration.Result{Migrated: 1}
	case errors.Is(err, &database.ErrConcurrency{}), errors.Is(err, &database.ErrNotFound{}):
		return removeUnusedCopy(ctx, client, logger, obj.ID, copied)
	default:
		// The copy is complete and readers use it. The next pass removes the legacy entry.
		return failed(logger, err)
	}
}

// removeUnusedCopy handles a legacy entry that changed or was deleted after the move read it. It deletes the copy the
// move saved, with the copy's ETag, so a copy that a request or another migrator changed since is kept.
//
// A legacy entry that changed, for example because a request queued an operation on it, remains the only entry, so
// no request sees an idle copy of a busy entry, and a later pass moves it once it is idle. A legacy entry is deleted
// only by the updater, when it finds the resource gone, or by a migrator that deduplicates it, which changes the
// current entry first. A deleted legacy entry and an unchanged copy therefore mean that the resource is gone and the
// copy would bring its entry back.
func removeUnusedCopy(ctx context.Context, client database.Client, logger logr.Logger, legacyID string, copied *database.Object) hashmigration.Result {
	_, err := client.Get(ctx, legacyID)
	legacyExists := err == nil
	if err != nil && !errors.Is(err, &database.ErrNotFound{}) {
		return failed(logger, err)
	}

	err = client.Delete(ctx, copied.ID, database.WithETag(copied.ETag))
	removed := err == nil
	if err != nil && !errors.Is(err, &database.ErrNotFound{}) && !errors.Is(err, &database.ErrConcurrency{}) {
		return failed(logger, err)
	}

	if legacyExists {
		// The next pass re-evaluates the legacy entry.
		return hashmigration.Result{Remaining: 1}
	}

	if removed {
		logger.Info("A tracked resource entry was deleted while it was moved; removed the entry the move saved under the SHA-256 tracking ID")
	}
	return hashmigration.Result{}
}

// changeSalt changes the salt of the tracking entry obj, with its ETag as a precondition, so that its ETag changes.
// The salt is never read.
func changeSalt(ctx context.Context, client database.Client, obj *database.Object) error {
	entry := datamodel.GenericResource{}
	if err := obj.As(&entry); err != nil {
		return err
	}

	entry.Properties.OperationID = uuid.New().String()
	return client.Save(ctx, &database.Object{ID: obj.ID, Data: &entry}, database.WithETag(obj.ETag))
}

func failed(logger logr.Logger, err error) hashmigration.Result {
	logger.Info("Unable to migrate a tracked resource entry to its SHA-256 tracking ID", "error", err.Error())
	return hashmigration.Result{Remaining: 1, Message: err.Error()}
}
