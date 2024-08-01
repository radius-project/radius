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

BICEP_RECIPE_TAG_VERSION?=latest

TERRAFORM_MODULE_SERVER_NAMESPACE=radius-test-tf-module-server
TERRAFORM_MODULE_SERVER_DEPLOYMENT_NAME=tf-module-server
TERRAFORM_MODULE_CONFIGMAP_NAME=tf-module-server-content

##@ Recipes

.PHONY: publish-test-bicep-recipes
publish-test-bicep-recipes: ## Publishes test recipes to <BICEP_RECIPE_REGISTRY> with version <BICEP_RECIPE_TAG_VERSION>
	@if [ -z "$(BICEP_RECIPE_REGISTRY)" ]; then echo "Error: BICEP_RECIPE_REGISTRY must be set to a valid OCI registry"; exit 1; fi
	
	@echo "$(ARROW) Publishing Bicep test recipes from ./test/testrecipes/test-bicep-recipes..."
	./.github/scripts/publish-recipes.sh \
		./test/testrecipes/test-bicep-recipes \
		${BICEP_RECIPE_REGISTRY}/test/testrecipes/test-bicep-recipes \
		${BICEP_RECIPE_TAG_VERSION}

.PHONY: publish-test-terraform-recipes
publish-test-terraform-recipes: ## Publishes test terraform recipes to the current Kubernetes cluster
	@echo "$(ARROW) Creating Kubernetes namespace $(TERRAFORM_MODULE_SERVER_NAMESPACE)..."
	kubectl create namespace $(TERRAFORM_MODULE_SERVER_NAMESPACE) --dry-run=client -o yaml | kubectl apply -f -

	@echo "$(ARROW) Publishing Terraform test recipes from ./test/testrecipes/test-terraform-recipes..."
	./.github/scripts/publish-test-terraform-recipes.py \
		./test/testrecipes/test-terraform-recipes \
		$(TERRAFORM_MODULE_SERVER_NAMESPACE) \
		$(TERRAFORM_MODULE_CONFIGMAP_NAME)
	
	@echo "$(ARROW) Deploying web server..."
	kubectl apply -f ./deploy/tf-module-server/resources.yaml -n $(TERRAFORM_MODULE_SERVER_NAMESPACE)

	@echo "$(ARROW) Waiting for web server to be ready..."
	kubectl rollout status deployment.apps/tf-module-server -n $(TERRAFORM_MODULE_SERVER_NAMESPACE) --timeout=600s

	@echo "$(ARROW) Web server ready. Recipes published to http://$(TERRAFORM_MODULE_SERVER_DEPLOYMENT_NAME).$(TERRAFORM_MODULE_SERVER_NAMESPACE).svc.cluster.local/<recipe_name>.zip"
	@echo "$(ARROW) To test use:"
	@echo "$(ARROW)     kubectl port-forward svc/$(TERRAFORM_MODULE_SERVER_DEPLOYMENT_NAME) 8999:80 -n $(TERRAFORM_MODULE_SERVER_NAMESPACE)"
	@echo "$(ARROW)     curl http://localhost:8999/<recipe-name>.zip --output <recipe-name>.zip"