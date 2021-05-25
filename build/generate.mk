# ------------------------------------------------------------
# Copyright (c) Microsoft Corporation.
# Licensed under the MIT License.
# ------------------------------------------------------------

##@ Generate (Code and Schema Generation)

.PHONY: generate
generate: generate-radclient generate-go ## Generates all targets.

.PHONY: generate-node-installed
generate-node-installed:
	@echo "$(ARROW) Detecting node..."
	@which node > /dev/null || { echo "node is a required dependency"; exit 1; }
	@echo "$(ARROW) OK"

.PHONY: generate-autorest-installed
generate-autorest-installed:
	@echo "$(ARROW) Detecting autorest..."
	@which autorest > /dev/null || { echo "run 'npm install -g autorest' to install autorest"; exit 1; }
	@echo "$(ARROW) OK"

.PHONY: generate-radclient
generate-radclient: generate-node-installed generate-autorest-installed ## Generates the radclient SDK (Autorest).
	autorest --use=@autorest/go@4.0.0-preview.14 \
		schemas/rest-api-specs/readme.md \
		--tag=package-2018-09-01-preview \
		--go  \
		--gomod-root=. \
		--output-folder=./pkg/radclient \
		--modelerfour.lenient-model-deduplication \
		--license-header=MICROSOFT_MIT_NO_VERSION \
		--file-prefix=zz_generated_ \
		--azure-arm \
		--verbose

.PHONY: generate-mockgen-installed
generate-mockgen-installed:
	@echo "$(ARROW) Detecting mockgen..."
	@which mockgen > /dev/null || { echo "run 'go install github.com/golang/mock/mockgen@v1.5.0' to install mockgen"; exit 1; }
	@echo "$(ARROW) OK"

.PHONY: generate-go
generate-go: generate-mockgen-installed ## Generates go with 'go generate' (Mocks).
	@echo "$(ARROW) Running go generate..."
	go generate -v ./... 
