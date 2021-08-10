# ------------------------------------------------------------
# Copyright (c) Microsoft Corporation.
# Licensed under the MIT License.
# ------------------------------------------------------------

# Assumptions:
# 1. kcp binary is located under ~/bin
# 2. kcp has been started (cd ~/bin && ./kcp start)
# 3. The resulting kcp configuration files are under ~/bin/.kcp

radiusd-run: check-kcp-running
	KUBECONFIG=~/bin/.kcp/data/admin.kubeconfig go run ./cmd/radiusd/main.go -zap-devel

radiusd-crd-install: check-kcp-running
	KUBECONFIG=~/bin/.kcp/data/admin.kubeconfig kubectl apply -f ./deploy/localdev/crds/radius.dev_executables.yaml

radiusd-crd-uninstall: check-kcp-running
	KUBECONFIG=~/bin/.kcp/data/admin.kubeconfig kubectl delete -f ./deploy/localdev/crds/radius.dev_executables.yaml

check-kcp-running:
	./build/check-kcp.sh
