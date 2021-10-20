# ------------------------------------------------------------
# Copyright (c) Microsoft Corporation.
# Licensed under the MIT License.
# ------------------------------------------------------------

##@ Controller

controller-run: generate-k8s-manifests generate-controller ## Run the controller locally
	SKIP_WEBHOOKS=true go run ./cmd/k8s/main.go

controller-crd-install: generate-k8s-manifests  ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	kubectl apply -f deploy/Chart/crds/

controller-crd-uninstall: generate-k8s-manifests  ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config.
	kubectl delete -f deploy/Chart/crds/

create-namespace: # Ignore failures from creating a namespace as it may alreadye exist.
	-kubectl create namespace radius-system

controller-deploy: docker-build-radius-controller docker-push-radius-controller controller-deploy-existing ## Deploy controller to the K8s cluster specified in ~/.kube/config.

controller-deploy-existing: generate-k8s-manifests create-namespace ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	helm upgrade --wait --install --set container=$(DOCKER_REGISTRY)/radius-controller --set tag=$(DOCKER_TAG_VERSION) radius deploy/Chart -n radius-system

controller-undeploy: generate-k8s-manifests ## Undeploy controller from the K8s cluster specified in ~/.kube/config.
	helm uninstall radius
