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

package v20250801preview

import (
	"context"

	"github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/corerp/frontend/controller/environments/v20250801preview/settingsref"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

// CleanupSettingsReferences is a delete filter that removes the environment's reference
// from any referenced TerraformSettings and BicepSettings resources when the environment is deleted.
func CleanupSettingsReferences(ctx context.Context, oldResource *datamodel.Environment_v20250801preview, options *controller.Options) (rest.Response, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	envID := oldResource.ID

	// Remove reference from TerraformSettings if specified
	if oldResource.Properties.TerraformSettings != "" {
		if err := settingsref.RemoveTerraformSettingsReference(ctx, options.DatabaseClient, oldResource.Properties.TerraformSettings, envID); err != nil {
			// Log error but don't fail deletion.
			// The settings resource might have already been deleted or there could be a transient error.
			// The ReferencedBy list is eventually consistent and can be cleaned up in a future reconciliation if needed.
			logger.Error(err, "Failed to remove environment reference from terraformSettings during environment deletion",
				"environmentID", envID,
				"terraformSettingsID", oldResource.Properties.TerraformSettings)
		}
	}

	// Remove reference from BicepSettings if specified
	if oldResource.Properties.BicepSettings != "" {
		if err := settingsref.RemoveBicepSettingsReference(ctx, options.DatabaseClient, oldResource.Properties.BicepSettings, envID); err != nil {
			// Log error but don't fail deletion.
			// The settings resource might have already been deleted or there could be a transient error.
			// The ReferencedBy list is eventually consistent and can be cleaned up in a future reconciliation if needed.
			logger.Error(err, "Failed to remove environment reference from bicepSettings during environment deletion",
				"environmentID", envID,
				"bicepSettingsID", oldResource.Properties.BicepSettings)
		}
	}

	return nil, nil
}
