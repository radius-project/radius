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

DIST_DIR ?= dist
IMAGES_DIR := $(DIST_DIR)/images
METRICS_DIR := $(DIST_DIR)/metrics

##@ Artifacts

.PHONY: artifacts
artifacts: docker-build docker-save-images build-metrics ## Build all artifacts needed for testing/release

.PHONY: docker-save-images
docker-save-images: ## Save Docker images to dist/images/*.tar
	@echo "$(ARROW) Saving Docker images to $(IMAGES_DIR)"
	@mkdir -p $(IMAGES_DIR)
	@$(foreach APP,$(APPS_MAP),$(eval $(call parseApp,$(APP))) \
		echo "  Saving $(DOCKER_REGISTRY)/$(NAME):$(DOCKER_TAG_VERSION) to $(NAME).tar"; \
		docker save -o $(IMAGES_DIR)/$(NAME).tar $(DOCKER_REGISTRY)/$(NAME):$(DOCKER_TAG_VERSION);)

.PHONY: docker-load-images
docker-load-images: ## Load Docker images from dist/images/*.tar
	@echo "$(ARROW) Loading Docker images from $(IMAGES_DIR)"
	@if [ ! -d "$(IMAGES_DIR)" ]; then \
		echo "Error: $(IMAGES_DIR) does not exist"; \
		exit 1; \
	fi
	@for tar_file in $(IMAGES_DIR)/*.tar; do \
		if [ -f "$$tar_file" ]; then \
			echo "  Loading $$tar_file"; \
			docker load -i "$$tar_file"; \
		fi; \
	done

.PHONY: build-metrics
build-metrics: ## Collect build metrics (duration, image count, size)
	@echo "$(ARROW) Collecting build metrics"
	@mkdir -p $(METRICS_DIR)
	@IMAGE_COUNT=$$(docker images --filter "reference=$(DOCKER_REGISTRY)/*:$(DOCKER_TAG_VERSION)" --format "{{.Repository}}:{{.Tag}}" | wc -l | tr -d ' '); \
	TOTAL_SIZE=$$(docker images --filter "reference=$(DOCKER_REGISTRY)/*:$(DOCKER_TAG_VERSION)" --format "{{.Size}}" | awk '{s+=$$1} END {print s}'); \
	TIMESTAMP=$$(date -u +"%Y-%m-%dT%H:%M:%SZ"); \
	echo "{" > $(METRICS_DIR)/metrics.json; \
	echo "  \"timestamp\": \"$$TIMESTAMP\"," >> $(METRICS_DIR)/metrics.json; \
	echo "  \"image_count\": $$IMAGE_COUNT," >> $(METRICS_DIR)/metrics.json; \
	echo "  \"total_size_mb\": \"$$TOTAL_SIZE\"," >> $(METRICS_DIR)/metrics.json; \
	echo "  \"docker_registry\": \"$(DOCKER_REGISTRY)\"," >> $(METRICS_DIR)/metrics.json; \
	echo "  \"docker_tag_version\": \"$(DOCKER_TAG_VERSION)\"," >> $(METRICS_DIR)/metrics.json; \
	echo "  \"git_commit\": \"$(GIT_COMMIT)\"," >> $(METRICS_DIR)/metrics.json; \
	echo "  \"rel_version\": \"$(REL_VERSION)\"" >> $(METRICS_DIR)/metrics.json; \
	echo "}" >> $(METRICS_DIR)/metrics.json; \
	echo "Build Metrics:" > $(METRICS_DIR)/metrics.txt; \
	echo "  Timestamp: $$TIMESTAMP" >> $(METRICS_DIR)/metrics.txt; \
	echo "  Image Count: $$IMAGE_COUNT" >> $(METRICS_DIR)/metrics.txt; \
	echo "  Total Size: $$TOTAL_SIZE MB" >> $(METRICS_DIR)/metrics.txt; \
	echo "  Docker Registry: $(DOCKER_REGISTRY)" >> $(METRICS_DIR)/metrics.txt; \
	echo "  Docker Tag: $(DOCKER_TAG_VERSION)" >> $(METRICS_DIR)/metrics.txt; \
	echo "  Git Commit: $(GIT_COMMIT)" >> $(METRICS_DIR)/metrics.txt; \
	echo "  Release Version: $(REL_VERSION)" >> $(METRICS_DIR)/metrics.txt; \
	cat $(METRICS_DIR)/metrics.txt
