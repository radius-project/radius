.PHONY: build-metrics

##@ Metrics

build-metrics: ## Build images and write dist/metrics/metrics.json and metrics.txt with timing and image stats.
# build-metrics: Run the standard image build and capture simple build metrics.
# Usage:
#   make build-metrics [DOCKER_REGISTRY=ghcr.io/radius-project/dev DOCKER_TAG_VERSION=local]
# Outputs:
#   - dist/metrics/metrics.json with fields:
#       build_start_utc, build_end_utc, build_duration_sec,
#       images_count, images_total_bytes, commit, tag
#   - dist/metrics/metrics.txt (human summary)
# Notes:
#   - This target does not push images. It relies on the same docker-build target used elsewhere.
#   - To compute sizes, it runs docker-save-images to materialize dist/images/*.tar.
#   - If images are already built, duration will reflect only the build invocation time.

METRICS_DIR := $(CURDIR)/dist/metrics
METRICS_JSON := $(METRICS_DIR)/metrics.json
METRICS_TXT := $(METRICS_DIR)/metrics.txt

# Reuse variables if already defined by other includes
DOCKER_REGISTRY ?=
DOCKER_TAG_VERSION ?=
GIT_COMMIT ?=

build-metrics:
	@START_TS=$$(date -u +%s); \
	START_ISO=$$(date -u +%Y-%m-%dT%H:%M:%SZ); \
	DRY=$$(printf '%s' '$(MAKEFLAGS)' | grep -q 'n' && echo 1 || echo 0); \
	if [ "$$DRY" = "1" ]; then \
	  echo "[metrics] dry-run detected (MAKEFLAGS contains 'n'); skipping build and writes"; \
	  exit 0; \
	fi; \
	mkdir -p $(METRICS_DIR); \
	echo "[metrics] starting build at $$START_ISO"; \
	if [ "$$METRICS_COLLECT_ONLY" != "1" ]; then \
	  $(MAKE) docker-build; \
	  $(MAKE) docker-save-images; \
	fi; \
	END_TS=$$(date -u +%s); \
	END_ISO=$$(date -u +%Y-%m-%dT%H:%M:%SZ); \
	DUR=$$((END_TS-START_TS)); \
	if [ "$$METRICS_COLLECT_ONLY" = "1" ]; then DUR=0; fi; \
	IMG_DIR="$(CURDIR)/dist/images"; \
	if [ -d "$$IMG_DIR" ]; then \
	  COUNT=$$(find "$$IMG_DIR" -type f -name '*.tar' | wc -l | tr -d ' '); \
	  if command -v stat >/dev/null 2>&1; then \
	    if stat -c%s /dev/null >/dev/null 2>&1; then \
	      BYTES=$$(find "$$IMG_DIR" -type f -name '*.tar' -exec stat -c%s {} \; 2>/dev/null | awk '{s+=$$1} END {print s+0}'); \
	    elif stat -f%z /dev/null >/dev/null 2>&1; then \
	      BYTES=$$(find "$$IMG_DIR" -type f -name '*.tar' -exec stat -f%z {} \; 2>/dev/null | awk '{s+=$$1} END {print s+0}'); \
	    else \
	      BYTES=$$(du -bc "$$IMG_DIR"/*.tar 2>/dev/null | tail -n1 | cut -f1 || echo 0); \
	    fi; \
	  else \
	    BYTES=$$(du -bc "$$IMG_DIR"/*.tar 2>/dev/null | tail -n1 | cut -f1 || echo 0); \
	  fi; \
	else \
	  COUNT=0; BYTES=0; \
	fi; \
	COMMIT=$${GIT_COMMIT:-$$(git rev-parse --short=12 HEAD 2>/dev/null || echo unknown)}; \
	TAG=$${DOCKER_TAG_VERSION:-$$(git rev-parse --short=12 HEAD 2>/dev/null || echo dev)}; \
	echo "{\"build_start_utc\":\"$$START_ISO\",\"build_end_utc\":\"$$END_ISO\",\"build_duration_sec\":$$DUR,\"images_count\":$$COUNT,\"images_total_bytes\":$$BYTES,\"commit\":\"$$COMMIT\",\"tag\":\"$$TAG\"}" > $(METRICS_JSON); \
	echo "Build metrics" > $(METRICS_TXT); \
	echo "  start: $$START_ISO" >> $(METRICS_TXT); \
	echo "  end:   $$END_ISO" >> $(METRICS_TXT); \
	echo "  duration(s): $$DUR" >> $(METRICS_TXT); \
	echo "  images: $$COUNT" >> $(METRICS_TXT); \
	echo "  bytes:  $$BYTES" >> $(METRICS_TXT); \
	echo "  commit: $$COMMIT" >> $(METRICS_TXT); \
	echo "  tag:    $$TAG" >> $(METRICS_TXT); \
	echo "[metrics] wrote $(METRICS_JSON) and $(METRICS_TXT)";
