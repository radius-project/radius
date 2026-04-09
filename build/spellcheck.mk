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

##@ Spellcheck

.PHONY: spellcheck
spellcheck: ## Runs spellcheck on the repository.
	@echo "$(ARROW) Running spellcheck..."
	@echo ""
	@command -v cspell >/dev/null 2>&1 || { \
		echo "cspell is required for spellcheck. Install it with 'npm install -g cspell', then try again."; \
		exit 1; \
	}
	@cspell lint --config ./.github/configs/.cspell.yml --no-progress --dot "**/*.md"
