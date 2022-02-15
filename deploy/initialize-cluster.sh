#!/usr/bin/env bash
set -eux

# Note: It's important that anything we in this script to mutate the runtime environment is idempotnent
# Ex: use `helm upgrade --install` instead of `helm install`
# 
# The script might run with retries before succeeding. It's OK to dirty the state of the container
# because each run has a separate container.

if [[ "$#" -ne 4 ]]
then
  echo "usage: initialize-cluster.sh <resource-group> <cluster-name> <chart-version> <image-tag>"
  exit 1
fi

RESOURCE_GROUP=$1
CLUSTER_NAME=$2
CHART_VERSION=$3
IMAGE_TAG=$4

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
  # see: https://github.com/project-radius/radius/issues/404
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

# Need to retry each command because the cluster might be in a state where the command fails
# We've seen a lot of failures in this area: 
for i in {1..5}
do
  echo "adding dapr helm repo - attempt $i"
  helm repo add dapr https://dapr.github.io/helm-charts/
  if [[ "$?" -eq 0 ]]
  then
    break
  fi
done

for i in {1..5}
do
  echo "adding haproxy-ingress helm repo - attempt $i"
  helm repo add haproxy-ingress https://haproxy-ingress.github.io/charts
  if [[ "$?" -eq 0 ]]
  then
    break
  fi
done

for i in {1..5}
do
  echo "adding radius helm repo - attempt $i"
  helm repo add radius https://radius.azurecr.io/helm/v1/repo
  if [[ "$?" -eq 0 ]]
  then
    break
  fi
done

for i in {1..5}
do
  echo "updating helm repos- attempt $i"
  helm repo update
  if [[ "$?" -eq 0 ]]
  then
    break
  fi
done

for i in {1..5}
do
  echo "installing dapr - attempt $i"
  helm upgrade \
    dapr dapr/dapr \
    --install \
    --create-namespace \
    --namespace dapr-system \
    --version 1.0.0 \
    --wait
  if [[ "$?" -eq 0 ]]
  then
    break
  fi
done

# Use retries when invoking kubectl - we've seen a crashes due to unexplained SIGBUS issues 
# ex: https://github.com/project-radius/radius/issues/29 https://github.com/project-radius/radius/issues/39
for i in {1..5}
do
  echo "listing dapr pods - attempt $i"
  if ./kubectl get pods -n dapr-system
  then
    break
  fi
done

for i in {1..5}
do
  echo "installing dapr - attempt $i"
  ./kubectl kustomize\
    "github.com/kubernetes-sigs/gateway-api/config/crd?ref=v0.3.0" |\
    ./kubectl apply -f -
  if [[ "$?" -eq 0 ]]
  then
    break
  fi
done

# Install haproxy-ingress
for i in {1..5}
do
  echo "listing dapr pods - attempt $i"
  cat <<EOF | helm upgrade --install haproxy-ingress haproxy-ingress/haproxy-ingress \
  --create-namespace --namespace radius-system \
  --version 0.13.4 \
  -f -
controller:
  hostNetwork: true
  extraArgs:
    watch-gateway: "true"
EOF
  if [[ "$?" -eq 0 ]]
  then
    break
  fi
done

for i in {1..5}
do
  echo "listing dapr pods - attempt $i"
  cat <<EOF | ./kubectl apply -f -
apiVersion: networking.x-k8s.io/v1alpha1
kind: GatewayClass
metadata:
  name: haproxy
spec:
  controller: haproxy-ingress.github.io/controller
EOF
  if [[ "$?" -eq 0 ]]
  then
    break
  fi
done


# Use retries when invoking kubectl - we've seen a crashes due to unexplained SIGBUS issues 
# ex: https://github.com/project-radius/radius/issues/29 https://github.com/project-radius/radius/issues/39
for i in {1..5}
do
  echo "listing radius-haproxy-ingress pods - attempt $i"
  if ./kubectl get pods -n radius-system
  then
    break
  fi
done

for i in {1..5}
do
  echo "installing radius runtime - attempt $i"
  helm upgrade \
    radius radius/radius \
    --install \
    --create-namespace \
    --namespace radius-system \
    --version $CHART_VERSION \
    --set tag=$IMAGE_TAG
    --wait
  if [[ "$?" -eq 0 ]]
  then
    break
  fi
done