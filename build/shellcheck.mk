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

##@ Shellcheck

# Shared ShellCheck configuration. The rc file is not auto-discovered because it
# lives under .github/linters/ rather than a script's directory, so it is passed
# explicitly with --rcfile.
SHELLCHECK_RCFILE := ./.github/linters/.shellcheckrc

# Paths excluded from shell linting, as an extended-regex matched against the
# repo-relative script path. .specify/ holds third-party Spec Kit tooling that is
# generated and not maintained in this repository.
SHELLCHECK_EXCLUDE_RE := ^\.specify/

.PHONY: lint-shell
lint-shell: ## Runs shellcheck static analysis on all tracked shell scripts.
	@command -v shellcheck >/dev/null 2>&1 || { \
		echo "shellcheck is required for lint-shell. Install the pinned version with 'make install-shellcheck', then try again."; \
		exit 1; \
	}
	@echo "$(ARROW) Running shellcheck..."
	@files=$$(git ls-files '*.sh' | grep -vE '$(SHELLCHECK_EXCLUDE_RE)'); \
	if [ -z "$$files" ]; then \
		echo "No shell scripts to lint."; \
	else \
		echo "$$files" | xargs shellcheck --rcfile $(SHELLCHECK_RCFILE); \
	fi
