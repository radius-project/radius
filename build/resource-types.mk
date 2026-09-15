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
# manifests from the resource-types-contrib repository and for maintaining the
# upstream pins recorded in deploy/manifest/defaults.yaml.
#
# resource-types-contrib contains only YAML manifests and HCL/Bicep recipes -
# no executable Go code. Rather than vendoring it as a Go module, the manifests
# are fetched directly from pinned upstream git revisions recorded per namespace
# in defaults.yaml (resourceTypes[].repo / resourceTypes[].ref).
#
# defaults.yaml carries two pin sections that share the entry shape
# {name, repo, ref, tag}:
#   resourceTypes  per-namespace pins for the manifests listed under
#                  `defaultRegistration`. build/scripts/sync-resource-types.sh
#                  resolves each default type to its namespace's entry, fetches
#                  each distinct (repo, ref) once, copies
#                  <namespace>/<typeName>/<typeName>.yaml (with the "Radius."
#                  prefix stripped) into every destination directory, and prunes
#                  stale managed files.
#   recipePacks    per-pack pins for the recipe packs published upstream. Recipe
#                  packs are not vendored here. Extension workflows resolve
#                  their immutable sources directly from this catalog.
#
# At startup UCP's RegisterDirectory loads the committed files unchanged;
# manifests without a "location" field are routed via DefaultDownstreamEndpoint
# (dynamic-rp).
#
# Targets:
#   update-resource-types-and-recipe-packs
#                          - Atomically apply stable-first selection to both pin
#                            sections, then copy the resource type manifests.
#   update-resource-types  - Select a stable release for each resourceTypes
#                            entry, falling back to its requested edge ref only
#                            when no stable release exists, then pin the commit
#                            SHA and copy the manifest files.
#   update-recipe-packs    - Apply the same stable-first selection to each
#                            recipePacks entry. Nothing is copied; only the pins
#                            are rewritten.
#   sync-resource-types    - Copy manifest files from the refs already pinned in
#                            defaults.yaml (no ref bump). Used by CI to verify
#                            manifest and pin drift.

# Path to the file listing default resource types and the upstream pins.
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

# Candidate ref that update-resource-types uses only when a namespace has no
# stable release. It must be "main" (the moving edge channel) or a full commit
# SHA and defaults to "main". A stable Radius.<Namespace>/vX.Y.Z tag always wins;
# prereleases are ignored. RESOURCE_TYPES_NAMESPACE optionally limits the
# requested update to one namespace; empty requests every namespace.
# RESOURCE_TYPES_PINS takes precedence and carries dispatch candidates for the
# affected namespaces. Existing edge pins are promoted whenever stable tags
# become available.
# Examples:
#   make update-resource-types
#   make update-resource-types RESOURCE_TYPES_REF=Radius.Compute/v0.2.0 RESOURCE_TYPES_NAMESPACE=Radius.Compute
RESOURCE_TYPES_REF ?= main
RESOURCE_TYPES_NAMESPACE ?=
RESOURCE_TYPES_PINS ?=
export RESOURCE_TYPES_REF RESOURCE_TYPES_NAMESPACE RESOURCE_TYPES_PINS

# Same stable-first contract as the RESOURCE_TYPES_* variables above, for the
# recipePacks section. Recipe packs are released upstream on their own
# recipe-pack/<pack>/vX.Y.Z tag series, so their pins advance independently.
# Examples:
#   make update-recipe-packs
#   make update-recipe-packs RECIPE_PACKS_REF=recipe-pack/azure/v0.2.0 RECIPE_PACKS_NAME=azure
RECIPE_PACKS_REF ?= main
RECIPE_PACKS_NAME ?=
RECIPE_PACKS_PINS ?=
export RECIPE_PACKS_REF RECIPE_PACKS_NAME RECIPE_PACKS_PINS

# Config consumed by build/scripts/sync-resource-types.sh (which does the fetch,
# copy, and prune). The RESOURCE_TYPES_* / RECIPE_PACKS_* variables reach the
# script through the environment via the exports above.
SYNC_RESOURCE_TYPES_ENV := \
	DEFAULTS_YAML="$(DEFAULTS_YAML)" \
	MANIFEST_DEST_DIRS="$(MANIFEST_DEST_DIRS)" \
	MANUAL_CORE_MANIFESTS="$(MANUAL_CORE_MANIFESTS)"

##@ Resource Types

.PHONY: update-resource-types-and-recipe-packs
update-resource-types-and-recipe-packs: ## Atomically pin stable resource types and recipe packs, then sync manifests
	@$(SYNC_RESOURCE_TYPES_ENV) ./build/scripts/sync-resource-types.sh --update-all

.PHONY: update-resource-types
update-resource-types: ## Pin stable resource type releases (edge only when unreleased) and sync manifests
	@$(SYNC_RESOURCE_TYPES_ENV) ./build/scripts/sync-resource-types.sh --update

.PHONY: update-recipe-packs
update-recipe-packs: ## Pin stable recipe pack releases (edge only when unreleased)
	@$(SYNC_RESOURCE_TYPES_ENV) ./build/scripts/sync-resource-types.sh --update-recipe-packs

.PHONY: sync-resource-types
sync-resource-types: ## Copy manifest files from the per-namespace refs pinned in defaults.yaml
	@$(SYNC_RESOURCE_TYPES_ENV) ./build/scripts/sync-resource-types.sh

.PHONY: test-sync-resource-types
test-sync-resource-types: ## Test stable-first resource type and recipe pack pin selection
	@bash ./build/scripts/test-sync-resource-types.sh
