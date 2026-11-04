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
// than lose the update. Every reader resolves the current ID first, so a legacy entry left behind by such a race is
// redundant and is removed on a later pass once it is idle. See https://github.com/radius-project/radius/issues/8084.
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

	_, err = client.Get(ctx, currentID.String())
	currentExists := err == nil
	if err != nil && !errors.Is(err, &database.ErrNotFound{}) {
		return failed(logger, err)
	}

	if !currentExists {
		moved := entry
		moved.ID = currentID.String()
		moved.Name = currentID.Name()
		err := client.Save(ctx, &database.Object{ID: currentID.String(), Data: &moved}, database.WithCreateOnly())
		if errors.Is(err, &database.ErrConcurrency{}) {
			// Another writer created the current entry after it was read. Keep it; the next pass removes the legacy entry.
			return hashmigration.Result{Remaining: 1}
		} else if err != nil {
			return failed(logger, err)
		}
	}

	err = client.Delete(ctx, obj.ID, database.WithETag(obj.ETag))
	switch {
	case errors.Is(err, &database.ErrConcurrency{}), errors.Is(err, &database.ErrNotFound{}):
		// The legacy entry changed or disappeared after it was read. The next pass re-evaluates it.
		return hashmigration.Result{Remaining: 1}
	case err != nil:
		return failed(logger, err)
	case currentExists:
		return hashmigration.Result{Deduplicated: 1}
	default:
		return hashmigration.Result{Migrated: 1}
	}
}

func failed(logger logr.Logger, err error) hashmigration.Result {
	logger.Info("Unable to migrate a tracked resource entry to its SHA-256 tracking ID", "error", err.Error())
	return hashmigration.Result{Remaining: 1, Message: err.Error()}
}
