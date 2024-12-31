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

##@ Formatting (of JSON files)

PRETTIER_VERSION := 3.3.3

.PHONY: format-check
format-check: ## Checks the formatting of JSON files.
	@echo "$(ARROW) Checking for formatting issues using prettier..."
	@echo ""
	@npx --yes prettier@$(PRETTIER_VERSION) --check "*/**/*.{ts,js,mjs,json}"

.PHONY: format-write
format-write: ## Updates the formatting of JSON files.
	@echo "$(ARROW) Reformatting files using prettier..."
	@echo ""
	@npx --yes prettier@$(PRETTIER_VERSION) --write "*/**/*.{ts,js,mjs,json}"
