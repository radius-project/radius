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

WORKFLOW_SCRIPT_DIR := build
WORKFLOW_SCRIPT := $(WORKFLOW_SCRIPT_DIR)/workflow-helper.sh

# Default branch to current branch if not specified
BRANCH ?= $(shell git rev-parse --abbrev-ref HEAD)

.PHONY: workflow-dispatch
workflow-dispatch: ## Dispatch a workflow by name. Usage: make workflow-dispatch NAME=<workflow-file> [BRANCH=<branch>]
	@if [ -z "$(NAME)" ]; then \
		echo "Error: NAME parameter is required. Usage: make workflow-dispatch NAME=<workflow-file> [BRANCH=<branch>]"; \
		exit 1; \
	fi
	@bash $(WORKFLOW_SCRIPT) dispatch "$(NAME)" "$(BRANCH)"

.PHONY: workflow
workflow: workflow-dispatch ## Alias for workflow-dispatch

.PHONY: workflow-list
workflow-list: ## List all workflows in the repository
	@bash $(WORKFLOW_SCRIPT) list

.PHONY: workflow-disable
workflow-disable: ## Disable a workflow by name. Usage: make workflow-disable NAME=<workflow-name>
	@if [ -z "$(WORKFLOW_NAME)" ]; then \
		echo "Error: WORKFLOW_NAME parameter is required. Usage: make workflow-disable WORKFLOW_NAME=<workflow file name>"; \
		exit 1; \
	fi
	@bash $(WORKFLOW_SCRIPT) disable "$(WORKFLOW_NAME)"

.PHONY: workflow-disable-triggered
workflow-disable-triggered: ## Disable all workflows triggered by events (push, PR, timers)
	@bash $(WORKFLOW_SCRIPT) disable-triggered

.PHONY: workflow-enable
workflow-enable: ## Enable a workflow by name. Usage: make workflow-enable NAME=<workflow-name>
	@if [ -z "$(WORKFLOW_NAME)" ]; then \
		echo "Error: WORKFLOW_NAME parameter is required. Usage: make workflow-enable WORKFLOW_NAME=<workflow-name>"; \
		exit 1; \
	fi
	@bash $(WORKFLOW_SCRIPT) enable "$(WORKFLOW_NAME)"

.PHONY: workflow-delete-all-runs
workflow-delete-all-runs: ## Delete all workflow runs in the repository
	@bash $(WORKFLOW_SCRIPT) delete-all-runs
