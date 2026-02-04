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

// Package settingsref provides shared helper functions for managing references
// between environments and settings resources (TerraformSettings and BicepSettings).
package settingsref

import (
	"context"
	"errors"
	"slices"

	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

const (
	// maxRetries is the maximum number of retries for handling ErrConcurrency.
	maxRetries = 3
)

// RemoveTerraformSettingsReference removes an environment ID from the ReferencedBy list of a TerraformSettings resource.
// This function handles optimistic concurrency by retrying on ErrConcurrency up to maxRetries times.
// If the settings resource doesn't exist (ErrNotFound), it returns nil as there's nothing to remove.
func RemoveTerraformSettingsReference(ctx context.Context, dbClient database.Client, settingsID, envID string) error {
	id, err := resources.ParseResource(settingsID)
	if err != nil {
		return err
	}

	for range maxRetries {
		obj, err := dbClient.Get(ctx, id.String())
		if err != nil {
			// If settings doesn't exist, nothing to remove
			if errors.Is(err, &database.ErrNotFound{}) {
				return nil
			}
			return err
		}

		settings := &datamodel.TerraformSettings_v20250801preview{}
		if err := obj.As(settings); err != nil {
			return err
		}

		// Remove envID from ReferencedBy
		newRefs := slices.DeleteFunc(settings.Properties.ReferencedBy, func(ref string) bool {
			return ref == envID
		})
		settings.Properties.ReferencedBy = newRefs

		obj.Data = settings
		err = dbClient.Save(ctx, obj, database.WithETag(obj.ETag))
		if err == nil {
			return nil
		}
		if errors.Is(err, &database.ErrConcurrency{}) {
			// Retry on concurrency conflict
			continue
		}
		return err
	}

	return &database.ErrConcurrency{}
}

// RemoveBicepSettingsReference removes an environment ID from the ReferencedBy list of a BicepSettings resource.
// This function handles optimistic concurrency by retrying on ErrConcurrency up to maxRetries times.
// If the settings resource doesn't exist (ErrNotFound), it returns nil as there's nothing to remove.
func RemoveBicepSettingsReference(ctx context.Context, dbClient database.Client, settingsID, envID string) error {
	id, err := resources.ParseResource(settingsID)
	if err != nil {
		return err
	}

	for range maxRetries {
		obj, err := dbClient.Get(ctx, id.String())
		if err != nil {
			// If settings doesn't exist, nothing to remove
			if errors.Is(err, &database.ErrNotFound{}) {
				return nil
			}
			return err
		}

		settings := &datamodel.BicepSettings_v20250801preview{}
		if err := obj.As(settings); err != nil {
			return err
		}

		// Remove envID from ReferencedBy
		newRefs := slices.DeleteFunc(settings.Properties.ReferencedBy, func(ref string) bool {
			return ref == envID
		})
		settings.Properties.ReferencedBy = newRefs

		obj.Data = settings
		err = dbClient.Save(ctx, obj, database.WithETag(obj.ETag))
		if err == nil {
			return nil
		}
		if errors.Is(err, &database.ErrConcurrency{}) {
			// Retry on concurrency conflict
			continue
		}
		return err
	}

	return &database.ErrConcurrency{}
}

// AddTerraformSettingsReference adds an environment ID to the ReferencedBy list of a TerraformSettings resource.
// This function handles optimistic concurrency by retrying on ErrConcurrency up to maxRetries times.
func AddTerraformSettingsReference(ctx context.Context, dbClient database.Client, settingsID, envID string) error {
	id, err := resources.ParseResource(settingsID)
	if err != nil {
		return err
	}

	for range maxRetries {
		obj, err := dbClient.Get(ctx, id.String())
		if err != nil {
			return err
		}

		settings := &datamodel.TerraformSettings_v20250801preview{}
		if err := obj.As(settings); err != nil {
			return err
		}

		// Add envID if not already present
		if slices.Contains(settings.Properties.ReferencedBy, envID) {
			return nil // Already referenced
		}

		settings.Properties.ReferencedBy = append(settings.Properties.ReferencedBy, envID)

		obj.Data = settings
		err = dbClient.Save(ctx, obj, database.WithETag(obj.ETag))
		if err == nil {
			return nil
		}
		if errors.Is(err, &database.ErrConcurrency{}) {
			// Retry on concurrency conflict
			continue
		}
		return err
	}

	return &database.ErrConcurrency{}
}

// AddBicepSettingsReference adds an environment ID to the ReferencedBy list of a BicepSettings resource.
// This function handles optimistic concurrency by retrying on ErrConcurrency up to maxRetries times.
func AddBicepSettingsReference(ctx context.Context, dbClient database.Client, settingsID, envID string) error {
	id, err := resources.ParseResource(settingsID)
	if err != nil {
		return err
	}

	for range maxRetries {
		obj, err := dbClient.Get(ctx, id.String())
		if err != nil {
			return err
		}

		settings := &datamodel.BicepSettings_v20250801preview{}
		if err := obj.As(settings); err != nil {
			return err
		}

		// Add envID if not already present
		if slices.Contains(settings.Properties.ReferencedBy, envID) {
			return nil // Already referenced
		}

		settings.Properties.ReferencedBy = append(settings.Properties.ReferencedBy, envID)

		obj.Data = settings
		err = dbClient.Save(ctx, obj, database.WithETag(obj.ETag))
		if err == nil {
			return nil
		}
		if errors.Is(err, &database.ErrConcurrency{}) {
			// Retry on concurrency conflict
			continue
		}
		return err
	}

	return &database.ErrConcurrency{}
}
