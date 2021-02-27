#!/usr/bin/env bash
set -eux

# Note: It's important that anything we in this script to mutate the runtime environment is idempotnent
# Ex: use `helm upgrade --install` instead of `helm install`
# 
# The script might run with retries before succeeding. It's OK to dirty the state of the container
# because each run has a separate container.

if [[ "$#" -ne 2 ]]
then
  echo "usage: initialize-cluster.sh <resource-group> <cluster-name>"
  exit 1
fi

RESOURCE_GROUP=$1
CLUSTER_NAME=$2

az aks get-credentials --name $CLUSTER_NAME --resource-group $RESOURCE_GROUP

KUBECTL_VERSION=$(curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt)

# Install kubectl - https://kubernetes.io/docs/tasks/tools/install-kubectl/#install-kubectl-on-linux
curl -LO "https://storage.googleapis.com/kubernetes-release/release/$KUBECTL_VERSION/bin/linux/amd64/kubectl"
chmod +x ./kubectl
./kubectl version --client

# Install helm - https://helm.sh/docs/intro/install/
curl https://raw.githubusercontent.com/helm/helm/master/scripts/get-helm-3 | bash
helm version

# Install Dapr CLI
wget -q https://raw.githubusercontent.com/dapr/cli/master/install/install.sh -O - | /bin/bash -s 1.0.0
dapr --version

# Install Dapr
helm repo add dapr https://dapr.github.io/helm-charts/
helm repo update

helm upgrade \
  dapr dapr/dapr \
  --install \
  --create-namespace \
  --namespace dapr-system \
  --version 1.0.0

./kubectl get pods -n dapr-system