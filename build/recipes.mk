# ------------------------------------------------------------
# Copyright (c) Microsoft Corporation.
# Licensed under the MIT License.
# ------------------------------------------------------------

RECIPE_TAG_VERSION?=latest
RAD_BICEP_PATH?=$${HOME}/.rad/bin

##@ Recipes

.PHONY: publish-test-recipes
publish-test-recipes: ## Publishes test recipes to <RECIPE_REGISTRY> with version <RECIPE_TAG_VERSION>
	@if [ -z "$(RECIPE_REGISTRY)" ]; then echo "Error: RECIPE_REGISTRY must be set to a valid OCI registry"; exit 1; fi
	
	@echo "$(ARROW) Publishing recipes from ./test/functional/corerp/resources/testdata/recipes/test-recipes..."
	./.github/scripts/publish-recipes.sh \
		${RAD_BICEP_PATH} \
		./test/functional/corerp/resources/testdata/recipes/test-recipes \
		${RECIPE_REGISTRY}/test/functional/corerp/recipes \
		${RECIPE_TAG_VERSION}