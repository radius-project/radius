#!/usr/bin/env bash
# Installs a Git HTTP backend backed by alpine/git and nginx.
set -euo pipefail

if [[ $# -lt 2 ]]; then
  echo "Usage: $0 <git-username> <git-password> [namespace] [image]" >&2
  exit 1
fi

SCRIPT_DIR=$(cd -- "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)

GIT_USERNAME=$1
GIT_PASSWORD=$2
NAMESPACE=${3:-git-http-backend}
IMAGE=${4:-${GIT_HTTP_IMAGE:-alpine/git:2.45.2}} # allow override via env or arg
SERVER_TEMP_DIR=${GIT_SERVER_TEMP_DIR:-/var/lib/git}

MANIFEST_TEMPLATE="${SCRIPT_DIR}/git-http-backend.yaml"
ENTRYPOINT_FILE="${SCRIPT_DIR}/git-http-backend-entrypoint.sh"
NGINX_TEMPLATE="${SCRIPT_DIR}/git-http-backend-nginx.conf"
WRAPPER_FILE="${SCRIPT_DIR}/git-http-backend-wrapper.sh"

for cmd in kubectl envsubst; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "error: required command '$cmd' not found in PATH" >&2
    exit 1
  fi
done

ensure_namespace() {
  kubectl create namespace "$1" --dry-run=client -o yaml | kubectl apply -f -
}

apply_basic_auth_secret() {
  kubectl -n "$1" create secret generic git-http-backend-auth \
    --type=kubernetes.io/basic-auth \
    --from-literal=username="$2" \
    --from-literal=password="$3" \
    --dry-run=client -o yaml | kubectl apply -f -
}

apply_configmap() {
  kubectl -n "$1" create configmap git-http-backend-config \
    --from-file=entrypoint.sh="$2" \
    --from-file=nginx.conf.template="$3" \
    --from-file=git-http-backend-wrapper.sh="$4" \
    --dry-run=client -o yaml | kubectl apply -f -
}

apply_workload() {
  export NAMESPACE IMAGE SERVER_TEMP_DIR
  envsubst '${NAMESPACE} ${IMAGE} ${SERVER_TEMP_DIR}' <"$1" | kubectl apply -f -
}

ensure_namespace "$NAMESPACE"
apply_basic_auth_secret "$NAMESPACE" "$GIT_USERNAME" "$GIT_PASSWORD"
apply_configmap "$NAMESPACE" "$ENTRYPOINT_FILE" "$NGINX_TEMPLATE" "$WRAPPER_FILE"
apply_workload "$MANIFEST_TEMPLATE"

kubectl rollout restart deployment/git-http-backend -n "$NAMESPACE" >/dev/null 2>&1 || true

kubectl rollout status deployment/git-http-backend -n "$NAMESPACE" --timeout=240s
