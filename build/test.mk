# ------------------------------------------------------------
# Copyright (c) Microsoft Corporation.
# Licensed under the MIT License.
# ------------------------------------------------------------

##@ Test

.PHONY: test
test: ## Runs unit tests.
	go test ./pkg/...

.PHONY: test-integration
test-integration: ## Runs integration tests
	go test ./test/integrationtests/... -timeout 3600s
