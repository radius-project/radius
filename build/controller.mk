# ------------------------------------------------------------
# Copyright (c) Microsoft Corporation.
# Licensed under the MIT License.
# ------------------------------------------------------------

##@ Controller

controller-run: generate-k8s-manifests generate-controller ## Run the controller locally
	SKIP_WEBHOOKS=true go run ./cmd/k8s/main.go

controller-install: generate-k8s-manifests  ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	kubectl apply -f deploy/Chart/crds/

controller-uninstall: generate-k8s-manifests  ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config.
	kubectl delete -f deploy/Chart/crds/

controller-deploy: generate-k8s-manifests docker-build docker-push ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	helm upgrade --wait --install --set container=$(DOCKER_REGISTRY)/radius-controller:$(DOCKER_TAG_VERSION) radius deploy/Chart -n radius-system

controller-undeploy: generate-k8s-manifests ## Undeploy controller from the K8s cluster specified in ~/.kube/config.
	helm uninstall radius
