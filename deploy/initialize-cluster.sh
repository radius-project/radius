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

KUBECTL_VERSION=$(curl -L -s https://dl.k8s.io/release/stable.txt)

# Install kubectl and verify the binary with retries
# https://kubernetes.io/docs/tasks/tools/install-kubectl/#install-kubectl-on-linux
for i in {1..5}
do
  echo "downloading kubectl version: $KUBECTL_VERSION - attempt $i"

  curl -LO "https://dl.k8s.io/$KUBECTL_VERSION/bin/linux/amd64/kubectl.sha256"
  curl -LO "https://dl.k8s.io/release/$KUBECTL_VERSION/bin/linux/amd64/kubectl"
  
  # on Busybox certain versions only support the `-c` shorthand
  # see: https://github.com/Azure/radius/issues/404
  if echo "$(<kubectl.sha256) kubectl" | sha256sum -c
  then
    echo "kubectl verified"
    break
  fi
done

chmod +x ./kubectl

# Install helm - https://helm.sh/docs/intro/install/
curl https://raw.githubusercontent.com/helm/helm/master/scripts/get-helm-3 | bash
helm version

# Install Dapr
helm repo add dapr https://dapr.github.io/helm-charts/
helm repo update

helm upgrade \
  dapr dapr/dapr \
  --install \
  --create-namespace \
  --namespace dapr-system \
  --version 1.0.0 \
  --wait

# Use retries when invoking kubectl - we've seen a crashes due to unexplained SIGBUS issues 
# ex: https://github.com/Azure/radius/issues/29 https://github.com/Azure/radius/issues/39
for i in {1..5}
do
  echo "listing dapr pods - attempt $i"
  if ./kubectl get pods -n dapr-system
  then
    break
  fi
done

# Install nginx-ingress
helm repo add haproxy-ingress https://haproxy-ingress.github.io/charts
helm repo update

cat <<EOF | helm upgrade haproxy-ingress haproxy-ingress/haproxy-ingress \
  --create-namespace --namespace radius-system \
  --version 0.13.4 \
  -f -
controller:
  hostNetwork: true
  extraArgs:
    watch-gateway: "true"
EOF

cat <<EOF | kubectl apply -f -
apiVersion: networking.x-k8s.io/v1alpha1
kind: GatewayClass
metadata:
  name: haproxy
spec:
  controller: haproxy-ingress.github.io/controller
EOF

# Use retries when invoking kubectl - we've seen a crashes due to unexplained SIGBUS issues 
# ex: https://github.com/Azure/radius/issues/29 https://github.com/Azure/radius/issues/39
for i in {1..5}
do
  echo "listing radius-haproxy-ingress pods - attempt $i"
  if ./kubectl get pods -n radius-system
  then
    break
  fi
done