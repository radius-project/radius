# ------------------------------------------------------------
# Copyright (c) Microsoft Corporation.
# Licensed under the MIT License.
# ------------------------------------------------------------

##@ Controller

controller-run: generate-k8s-manifests generate-controller ## Run the controller locally
	SKIP_WEBHOOKS=true go run ./cmd/k8s/main.go

controller-install: generate-k8s-manifests ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	helm upgrade --wait --install --set container=$(DOCKER_REGISTRY)/radius-controller:$(DOCKER_TAG_VERSION) $(DOCKER_TAG_VERSION) cmd/cli/cmd/Chart/templates/

controller-uninstall: generate-k8s-manifests ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config.
	helm uninstall $(DOCKER_TAG_VERSION)

# controller-deploy: generate-k8s-manifests generate-kustomize-installed ## Deploy controller to the K8s cluster specified in ~/.kube/config.
# 	cd deploy/k8s/config/manager && $(KUSTOMIZE) edit set image controller=${DOCKER_TAG_VERSION}
# 	$(KUSTOMIZE) build cmd/cli/cmd/Chart/templates/ | kubectl apply -f -

# controller-undeploy: generate-kustomize-installed ## Undeploy controller from the K8s cluster specified in ~/.kube/config.
# 	$(KUSTOMIZE) build deploy/k8s/config/default | kubectl delete -f -