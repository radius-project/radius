# ------------------------------------------------------------
# Copyright (c) Microsoft Corporation.
# Licensed under the MIT License.
# ------------------------------------------------------------

##@ Controller

kubernetes-deploy: docker-build-radius-rp docker-push-radius-rp controller-deploy-existing ## Deploy controller to the K8s cluster specified in ~/.kube/config.

kubernetes-deploy-existing: generate-k8s-manifests ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	go run ./cmd/rad/main.go install kubernetes --reinstall --chart deploy/Chart/ --image $(DOCKER_REGISTRY)/radius-rp --tag $(DOCKER_TAG_VERSION) 

kubernetes-undeploy: ## Uninstall controller from the K8s cluster specified in ~/.kube/config.
	go run ./cmd/rad/main.go uninstall kubernetes
