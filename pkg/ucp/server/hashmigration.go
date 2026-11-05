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

package server

import (
	"context"

	"github.com/radius-project/radius/pkg/components/database/databaseprovider"
	"github.com/radius-project/radius/pkg/ucp/trackedresource"
	"github.com/radius-project/radius/pkg/upgrade/hashmigration"
)

const (
	// hashMigrationComponent is the component name UCP records its migration status under.
	hashMigrationComponent = "ucp"

	// trackedResourcesStep re-keys tracked resource entries.
	trackedResourcesStep = "trackedResources"

	// datastoreStep moves entries of the apiserver database provider to their current object names.
	datastoreStep = "datastore"
)

// legacyNameMigrator is implemented by database clients that store entries under names derived from a hash.
type legacyNameMigrator interface {
	MigrateLegacyNames(ctx context.Context) (hashmigration.Result, error)
}

// newHashMigrationService creates the service that migrates the SHA-1-derived values UCP owns: tracked resource IDs
// and, with the apiserver database provider, the object names of the data store every component shares.
//
// With exclusion, one UCP replica migrates at a time, which lets both steps remove a copy they wrote when a delete
// overtook the move. See hashmigration.IsExclusive.
func newHashMigrationService(provider *databaseprovider.DatabaseProvider, reporter hashmigration.Reporter, exclusion hashmigration.Exclusion) *hashmigration.Service {
	return hashmigration.NewService(hashmigration.Options{
		Component:    hashMigrationComponent,
		Steps:        hashMigrationSteps(provider),
		Reporter:     reporter,
		Exclusion:    exclusion,
		InitialDelay: hashmigration.DefaultInitialDelay,
	})
}

func hashMigrationSteps(provider *databaseprovider.DatabaseProvider) []hashmigration.Step {
	return []hashmigration.Step{
		// Tracked resources run first: re-keying an entry also moves it to its current object name, so the
		// data store step has less to move.
		hashmigration.StepFunc{
			StepName: trackedResourcesStep,
			Fn: func(ctx context.Context) (hashmigration.Result, error) {
				client, err := provider.GetClient(ctx)
				if err != nil {
					return hashmigration.Result{}, err
				}

				return trackedresource.MigrateLegacyEntries(ctx, client)
			},
		},
		hashmigration.StepFunc{
			StepName: datastoreStep,
			Fn: func(ctx context.Context) (hashmigration.Result, error) {
				client, err := provider.GetClient(ctx)
				if err != nil {
					return hashmigration.Result{}, err
				}

				migrator, ok := client.(legacyNameMigrator)
				if !ok {
					return hashmigration.Result{NotApplicable: true, Message: "the database provider does not derive storage names from hashes"}, nil
				}

				return migrator.MigrateLegacyNames(ctx)
			},
		},
	}
}
