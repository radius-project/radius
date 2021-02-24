#!/usr/bin/env bash
set -eux

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

# Install Dapr
wget -q https://raw.githubusercontent.com/dapr/cli/master/install/install.sh -O - | /bin/bash -s 1.0.0-rc.2
dapr --version
helm repo add dapr https://dapr.github.io/helm-charts/
helm repo update
cat <<EOF | ./kubectl apply -f -
apiVersion: v1
kind: Namespace
metadata:
  name: dapr-system
EOF
helm install dapr dapr/dapr --namespace dapr-system --version 1.0.0-rc.2
./kubectl get pods -n dapr-system