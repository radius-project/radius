#!/usr/bin/env bash
# Ensure the radius-encryption-key Secret exists in the radius-system namespace
# of the k3d-radius-debug cluster. dynamic-rp (and other Radius components that
# use the encryption key provider) refuse to start without it.
#
# The Helm chart (deploy/Chart/templates/dynamic-rp/secret.yaml) normally
# creates this. The OS-process debug stack skips Helm, so we recreate the same
# secret format here.
#
# This script is idempotent: if the secret already exists, it does nothing.

set -euo pipefail

CONTEXT="${KUBE_CONTEXT:-k3d-radius-debug}"
NAMESPACE="${RADIUS_NAMESPACE:-radius-system}"
SECRET_NAME="radius-encryption-key"

if ! command -v kubectl >/dev/null 2>&1; then
  echo "❌ kubectl not found"
  exit 1
fi

if ! kubectl --context "$CONTEXT" cluster-info >/dev/null 2>&1; then
  echo "❌ cluster $CONTEXT not reachable"
  exit 1
fi

# Ensure namespace exists
kubectl --context "$CONTEXT" get namespace "$NAMESPACE" >/dev/null 2>&1 \
  || kubectl --context "$CONTEXT" create namespace "$NAMESPACE" >/dev/null

# If the secret already exists, leave it alone.
if kubectl --context "$CONTEXT" -n "$NAMESPACE" get secret "$SECRET_NAME" >/dev/null 2>&1; then
  echo "✅ Secret $NAMESPACE/$SECRET_NAME already exists"
  exit 0
fi

# Generate a 32-byte random key and build the JSON keystore the same way the
# Helm chart does.
now_iso="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
expiry_iso="$(date -u -v+90d +%Y-%m-%dT%H:%M:%SZ 2>/dev/null \
              || date -u -d '+90 days' +%Y-%m-%dT%H:%M:%SZ)"
key_b64="$(head -c 32 /dev/urandom | base64 | tr -d '\n')"

keystore_json=$(cat <<EOF
{"currentVersion":1,"keys":{"1":{"key":"${key_b64}","version":1,"createdAt":"${now_iso}","expiresAt":"${expiry_iso}"}}}
EOF
)

kubectl --context "$CONTEXT" -n "$NAMESPACE" create secret generic "$SECRET_NAME" \
  --from-literal=keys.json="$keystore_json" >/dev/null

echo "✅ Created secret $NAMESPACE/$SECRET_NAME (random 32-byte key, 90-day expiry)"
