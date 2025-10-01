#!/usr/bin/env bash
# Sets up a KinD cluster suitable for Radius functional tests.
set -euo pipefail

CLUSTER_NAME=${CLUSTER_NAME:-radius}
KIND_IMAGE=${KIND_IMAGE:-}
PORT_HOST=${PORT_HOST:-30080}
PORT_CONTAINER=${PORT_CONTAINER:-30080}

KIND_CONFIG=$(mktemp)
cat <<CONFIG >"${KIND_CONFIG}"
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  extraPortMappings:
    - containerPort: ${PORT_CONTAINER}
      hostPort: ${PORT_HOST}
      protocol: TCP
CONFIG

if kind get clusters | grep -qx "${CLUSTER_NAME}"; then
  kind delete cluster --name "${CLUSTER_NAME}"
fi

if [[ -n "${KIND_IMAGE}" ]]; then
  kind create cluster --name "${CLUSTER_NAME}" --image "${KIND_IMAGE}" --config "${KIND_CONFIG}"
else
  kind create cluster --name "${CLUSTER_NAME}" --config "${KIND_CONFIG}"
fi

rm -f "${KIND_CONFIG}"
