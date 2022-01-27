# ------------------------------------------------------------
# Copyright (c) Microsoft Corporation.
# Licensed under the MIT License.
# ------------------------------------------------------------

##@ Controller

controller-run: generate-k8s-manifests generate-controller ## Run the controller locally
	SKIP_WEBHOOKS=true go run ./cmd/radius-controller/main.go

controller-crd-install: generate-k8s-manifests  ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	kubectl kustomize "github.com/kubernetes-sigs/gateway-api/config/crd?ref=v0.3.0" | kubectl apply -f -
	
	kubectl apply -f deploy/Chart/crds/
	kubectl wait --for condition="established" -f deploy/Chart/crds/

controller-crd-uninstall: generate-k8s-manifests  ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config.
	kubectl delete -f deploy/Chart/crds/

controller-deploy: docker-build-radius-controller docker-push-radius-controller controller-deploy-existing ## Deploy controller to the K8s cluster specified in ~/.kube/config.

controller-deploy-existing: generate-k8s-manifests ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	go run ./cmd/rad/main.go env init kubernetes -c deploy/Chart/ --image $(DOCKER_REGISTRY)/radius-controller --tag $(DOCKER_TAG_VERSION) 

controller-undeploy: generate-k8s-manifests ## Undeploy controller from the K8s cluster specified in ~/.kube/config.
	helm uninstall radius -n radius-system