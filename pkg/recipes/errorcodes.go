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

package recipes

const (
	// Used for recipe download failures.
	RecipeDownloadFailed = "RecipeDownloadFailed"

	// Used for recipe deployment failures.
	RecipeDeploymentFailed = "RecipeDeploymentFailed"

	// Used for recipe validation failures.
	RecipeValidationFailed = "RecipeValidationFailed"

	// Used for recipe deletion failures.
	RecipeDeletionFailed = "RecipeDeletionFailed"

	// Used when deletion can't confirm which resources a failed recipe deployment created. A forced delete skips this check.
	RecipeCleanupInventoryUnavailable = "RecipeCleanupInventoryUnavailable"

	// Used for errors encountered during processing recipe outputs.
	InvalidRecipeOutputs = "InvalidRecipeOutputs"

	// Used for errors encountered while reading a recipe from registry.
	RecipeLanguageFailure = "RecipeLanguageFailure"

	// Used for errors encountered while cleaning up of obsolete resources during patch operation.
	RecipeGarbageCollectionFailed = "RecipeGarbageCollectionFailed"

	// Used for errors encountered when getting recipe parameters.
	RecipeGetMetadataFailed = "RecipeGetMetadataFailed"

	// Used for errors when checking the existence of a recipe.
	RecipeNotFoundFailure = "RecipeNotFoundFailure"

	// Used for errors with recipe drivers
	RecipeDriverNotFoundFailure = "DriverNotFoundFailure"

	// Used for errors with recipe configuration
	RecipeConfigurationFailure = "RecipeConfigurationFailure"

	// Used for errors encountered while loading recipe secrets.
	LoadSecretsFailed = "LoadSecretsFailed"
)

// ForceDeleteHint tells users how to finish a delete that is blocked by unknown recipe cleanup inventory.
const ForceDeleteHint = "To delete anyway, check for leftover resources by hand, then delete again with force (--force in rad, or force=true on the DELETE request)."
