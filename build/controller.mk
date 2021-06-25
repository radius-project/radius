# ------------------------------------------------------------
# Copyright (c) Microsoft Corporation.
# Licensed under the MIT License.
# ------------------------------------------------------------

##@ Controller

controller-run: generate-k8s-manifests generate-controller ## Run the controller locally
	SKIP_WEBHOOKS=true go run ./cmd/k8s/main.go

controller-install: generate-k8s-manifests
	kubectl apply -f cmd/cli/cmd/Chart/templates/applications.radius.dev_applications.yaml \
		-f cmd/cli/cmd/Chart/templates/applications.radius.dev_components.yaml \
		-f cmd/cli/cmd/Chart/templates/applications.radius.dev_deployments.yaml
controller-uninstall: generate-k8s-manifests
	kubectl delete -f cmd/cli/cmd/Chart/templates/applications.radius.dev_applications.yaml \
		-f cmd/cli/cmd/Chart/templates/applications.radius.dev_components.yaml \
		-f cmd/cli/cmd/Chart/templates/applications.radius.dev_deployments.yaml

controller-deploy: generate-k8s-manifests docker-build docker-push ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	helm upgrade --wait --install --set container=$(DOCKER_REGISTRY)/radius-controller:$(DOCKER_TAG_VERSION) $(DOCKER_TAG_VERSION) cmd/cli/cmd/Chart/

controller-undeploy: generate-k8s-manifests ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config.
	helm uninstall $(DOCKER_TAG_VERSION)
