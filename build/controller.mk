# ------------------------------------------------------------
# Copyright (c) Microsoft Corporation.
# Licensed under the MIT License.
# ------------------------------------------------------------

##@ Controller

controller-run: generate-k8s-manifests generate-controller ## Run the controller locally
	go run ./cmd/k8s/main.go

controller-install: generate-k8s-manifests generate-kustomize-installed ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build deploy/k8s/config/crd | kubectl apply -f -

controller-uninstall: generate-k8s-manifests generate-kustomize-installed ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build deploy/k8s/config/crd | kubectl delete -f -

controller-deploy: generate-k8s-manifests generate-kustomize-installed ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd deploy/k8s/config/manager && $(KUSTOMIZE) edit set image controller=${K8S_IMAGE}
	$(KUSTOMIZE) build deploy/k8s/config/default | kubectl apply -f -

controller-undeploy: generate- ## Undeploy controller from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build deploy/k8s/config/default | kubectl delete -f -
