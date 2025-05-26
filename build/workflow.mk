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

##@ GitHub Workflows

WORKFLOW_SCRIPT := ./build/workflow.sh

.PHONY: workflow-disable-all
workflow-disable-all: ## Disable all workflows in the current repo
	@bash $(WORKFLOW_SCRIPT) disable-all

.PHONY: workflow-enable-all
workflow-enable-all: ## Enable all workflows in the current repo
	@bash $(WORKFLOW_SCRIPT) enable-all

.PHONY: workflow-delete-all-runs
workflow-delete-all-runs: ## Delete all workflow runs in the repository. NOTE: This is a destructive operation and cannot be undone.
	@bash $(WORKFLOW_SCRIPT) delete-all-runs
