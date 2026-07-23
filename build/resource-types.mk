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
# resource-types-contrib contains only YAML manifests and HCL/Bicep recipes -
# no executable Go code. Rather than vendoring it as a Go module, the manifests
# are fetched directly from pinned upstream git revisions recorded per namespace
# in deploy/manifest/defaults.yaml (sources[].repo / sources[].ref).
#
# defaults.yaml pins each namespace independently under `sources` and lists the
# default types under `defaultRegistration`. build/scripts/sync-resource-types.sh
# resolves each default type to its namespace's source, fetches each distinct
# (repo, ref) once, copies <namespace>/<typeName>/<typeName>.yaml (with the
# "Radius." prefix stripped) into every destination directory, and prunes stale
# managed files. At startup UCP's RegisterDirectory loads the committed files
# unchanged; manifests without a "location" field are routed via
# DefaultDownstreamEndpoint (dynamic-rp).
#
# Targets:
#   update-resource-types  - Resolve each source's ref (RESOURCE_TYPES_REF,
#                            default "main"; RESOURCE_TYPES_NAMESPACE limits to a
#                            single namespace) to an immutable commit SHA, pin it
#                            in defaults.yaml, and copy the manifest files.
#   sync-resource-types    - Copy manifest files from the refs already pinned in
#                            defaults.yaml (no ref bump). Used by CI to verify
#                            that committed copies match the pinned refs.

# Path to the file listing default resource types and the per-namespace pins.
DEFAULTS_YAML := deploy/manifest/defaults.yaml

# Directories where manifest copies are placed. Both directories contain the
# same set of files; dev/ is used for local development (endpoints point to
# localhost) and self-hosted/ is used for Kubernetes deployments.
# Note: The copied manifests themselves have no "location" field. The location
# is only present in the manually maintained files (radius_core.yaml, etc.).
MANIFEST_DEST_DIRS := deploy/manifest/built-in-providers/dev deploy/manifest/built-in-providers/self-hosted

# Files in the manifest destination directories that are manually maintained
# and should NOT be managed (created or deleted) by the sync target. These are
# resource providers that require explicit location addresses and are not
# sourced from resource-types-contrib.
MANUAL_CORE_MANIFESTS := applications_core.yaml applications_dapr.yaml applications_datastores.yaml applications_messaging.yaml microsoft_resources.yaml radius_core.yaml

# Ref (branch, tag, or commit SHA) that update-resource-types resolves to an
# immutable commit SHA before pinning it in defaults.yaml. Defaults to "main"
# (the moving latest/edge channel). RESOURCE_TYPES_NAMESPACE optionally limits
# the update to a single namespace (e.g. Radius.Compute); empty updates every
# namespace. RESOURCE_TYPES_PINS (a JSON array of {namespace, ref}) takes
# precedence and pins several namespaces at once - it is how the
# resource-types-contrib dispatch payload advances only the affected namespaces.
# Examples:
#   make update-resource-types
#   make update-resource-types RESOURCE_TYPES_REF=v0.56.0
#   make update-resource-types RESOURCE_TYPES_NAMESPACE=Radius.Compute RESOURCE_TYPES_REF=v0.56.0
RESOURCE_TYPES_REF ?= main
RESOURCE_TYPES_NAMESPACE ?=
RESOURCE_TYPES_PINS ?=
export RESOURCE_TYPES_REF RESOURCE_TYPES_NAMESPACE RESOURCE_TYPES_PINS

# Config consumed by build/scripts/sync-resource-types.sh (which does the fetch,
# copy, and prune). RESOURCE_TYPES_REF / RESOURCE_TYPES_NAMESPACE / RESOURCE_TYPES_PINS
# reach the script through the environment via the export above.
SYNC_RESOURCE_TYPES_ENV := \
	DEFAULTS_YAML="$(DEFAULTS_YAML)" \
	MANIFEST_DEST_DIRS="$(MANIFEST_DEST_DIRS)" \
	MANUAL_CORE_MANIFESTS="$(MANUAL_CORE_MANIFESTS)"

##@ Resource Types

.PHONY: update-resource-types
update-resource-types: ## Resolve each namespace source ref (RESOURCE_TYPES_REF, default main) to a commit SHA, pin it in defaults.yaml, and sync
	@$(SYNC_RESOURCE_TYPES_ENV) ./build/scripts/sync-resource-types.sh --update

.PHONY: sync-resource-types
sync-resource-types: ## Copy manifest files from the per-namespace refs pinned in defaults.yaml
	@$(SYNC_RESOURCE_TYPES_ENV) ./build/scripts/sync-resource-types.sh
