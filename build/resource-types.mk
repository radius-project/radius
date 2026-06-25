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

# resource-types.mk provides targets for synchronizing default resource type
# manifests from the resource-types-contrib repository.
#
# resource-types-contrib is added to go.mod as a Go module dependency. It
# contains no executable Go code. We depend on it purely to leverage Go's
# module system for versioned downloads of YAML manifest files.
#
# A blank import in pkg/resourcetypescontrib/import.go keeps go mod tidy from
# removing the dependency.
#
# How it works:
#   1. defaults.yaml lists which resource types to ship as defaults, using
#      <namespace>/<typeName> names (e.g. Radius.Compute/containers).
#   2. Each entry is resolved to a file path in the Go module cache:
#        Radius.Compute/containers → Compute/containers/containers.yaml
#      (strip "Radius." prefix, then <namespace>/<typeName>/<typeName>.yaml)
#   3. The resolved file is copied into both dev/ and self-hosted/ directories
#      under deploy/manifest/built-in-providers/.
#   4. At startup, UCP's RegisterDirectory loads these files. Manifests without
#      a "location" field are routed via DefaultDownstreamEndpoint (dynamic-rp).
#
# Targets:
#   update-resource-types  - Bump go.mod to the latest resource-types-contrib
#                            version and copy the manifest files.
#   sync-resource-types    - Copy manifest files from the version already pinned
#                            in go.mod (no version bump). Used by CI to verify
#                            that committed copies match the pinned version.

# Path to the file listing default resource types.
DEFAULTS_YAML := deploy/manifest/defaults.yaml

# Directories where manifest copies are placed. Both directories contain the
# same set of files; dev/ is used for local development (endpoints point to
# localhost) and self-hosted/ is used for Kubernetes deployments.
# Note: The copied manifests themselves have no "location" field. The location
# is only present in the manually maintained files (radius_core.yaml, etc.).
MANIFEST_DEST_DIRS := deploy/manifest/built-in-providers/dev deploy/manifest/built-in-providers/self-hosted

# The Go module path for resource-types-contrib.
RESOURCE_TYPES_MODULE := github.com/radius-project/resource-types-contrib

# Files in the manifest destination directories that are manually maintained
# and should NOT be managed (created or deleted) by the sync target. These are
# resource providers that require explicit location addresses and are not
# sourced from resource-types-contrib.
MANUAL_CORE_MANIFESTS := applications_core.yaml applications_dapr.yaml applications_datastores.yaml applications_messaging.yaml microsoft_resources.yaml radius_core.yaml

##@ Resource Types

.PHONY: update-resource-types
update-resource-types: ## Bump resource-types-contrib to latest and sync manifest files
	@echo "Updating $(RESOURCE_TYPES_MODULE) to latest version..."
	# Update only the resource-types-contrib dependency in go.mod to the latest
	# version. Using @latest (without -u) avoids upgrading transitive dependencies.
	go get $(RESOURCE_TYPES_MODULE)@latest
	go mod tidy
	# Copy the manifest files from the newly pinned version.
	$(MAKE) sync-resource-types

.PHONY: sync-resource-types
sync-resource-types: ## Copy manifest files listed in defaults.yaml from the pinned resource-types-contrib version
	@# Verify required tools are available before making any changes.
	@command -v yq >/dev/null 2>&1 || { echo "ERROR: yq is required but not found. Install via: make install-yq"; exit 1; }
	@command -v jq >/dev/null 2>&1 || { echo "ERROR: jq is required but not found. Install via: brew install jq (macOS) or apt-get install jq (Linux)"; exit 1; }
	@echo "Syncing default resource types from resource-types-contrib..."
	@# Resolve the module's local cache directory from the version pinned in
	@# go.mod. "go mod download -json" outputs JSON with a "Dir" field pointing
	@# to the cached module source on disk. Then iterate over each entry in
	@# defaults.yaml, convert the resource type name to a module-relative path
	@# (e.g. Radius.Compute/containers -> Compute/containers/containers.yaml),
	@# and copy the file into each destination directory (dev/ and self-hosted/).
	@MODULE_DIR=$$(go mod download -json $(RESOURCE_TYPES_MODULE) | jq -r '.Dir') && \
	if [ -z "$$MODULE_DIR" ] || [ "$$MODULE_DIR" = "null" ]; then \
		echo "ERROR: Could not resolve module directory for $(RESOURCE_TYPES_MODULE)."; \
		echo "       Is the dependency present in go.mod?"; \
		exit 1; \
	fi && \
	echo "  Module directory: $$MODULE_DIR" && \
	for entry in $$(yq '.defaultRegistration[]' $(DEFAULTS_YAML)); do \
		rel_path=$$(echo "$$entry" | sed 's/^Radius\.//') && \
		type_name=$$(echo "$$rel_path" | cut -d'/' -f2) && \
		src_path="$$MODULE_DIR/$$rel_path/$$type_name.yaml" && \
		if [ ! -f "$$src_path" ]; then \
			echo "ERROR: File not found: $$src_path (from entry '$$entry')"; \
			echo "       Verify the entry in $(DEFAULTS_YAML) and the resource-types-contrib version."; \
			exit 1; \
		fi && \
		for dest_dir in $(MANIFEST_DEST_DIRS); do \
			cp "$$src_path" "$$dest_dir/$$type_name.yaml"; \
		done && \
		echo "  Copied $$entry"; \
	done
	@# Remove stale managed files: any YAML in the destination directories that
	@# is NOT in MANUAL_CORE_MANIFESTS and NOT in the current defaults.yaml list.
	@# This prevents previously-copied manifests from remaining registered after
	@# their entry is removed from defaults.yaml.
	@EXPECTED_FILES="" && \
	for entry in $$(yq '.defaultRegistration[]' $(DEFAULTS_YAML)); do \
		rel_path=$$(echo "$$entry" | sed 's/^Radius\.//') && \
		type_name=$$(echo "$$rel_path" | cut -d'/' -f2) && \
		EXPECTED_FILES="$$EXPECTED_FILES $$type_name.yaml"; \
	done && \
	for dest_dir in $(MANIFEST_DEST_DIRS); do \
		for file in "$$dest_dir"/*.yaml; do \
			basename=$$(basename "$$file") && \
			is_manual=false && \
			for mc in $(MANUAL_CORE_MANIFESTS); do \
				if [ "$$basename" = "$$mc" ]; then is_manual=true; break; fi; \
			done && \
			if [ "$$is_manual" = "true" ]; then continue; fi && \
			is_expected=false && \
			for ef in $$EXPECTED_FILES; do \
				if [ "$$basename" = "$$ef" ]; then is_expected=true; break; fi; \
			done && \
			if [ "$$is_expected" = "false" ]; then \
				echo "  Removing stale manifest: $$file"; \
				rm "$$file"; \
			fi; \
		done; \
	done
	@echo "Done. Review and commit the updated files."
