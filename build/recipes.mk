# ------------------------------------------------------------
# Copyright 2023 The Radius Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#    
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
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