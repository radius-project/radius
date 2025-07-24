#!/bin/bash
set -e

echo "🚀 Starting deployment engine (Docker)..."

# Stop existing container if running
docker stop radius-deployment-engine 2>/dev/null || true
docker rm radius-deployment-engine 2>/dev/null || true

# Create kubeconfig for container access
echo "📝 Preparing kubeconfig for container..."
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEMP_KUBECONFIG="/tmp/radius-debug-kubeconfig"

# Get current kubeconfig and extract the needed parts
CURRENT_SERVER=$(kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}')
CURRENT_CERT_DATA=$(kubectl config view --raw --minify -o jsonpath='{.clusters[0].cluster.certificate-authority-data}')
CURRENT_CLIENT_CERT=$(kubectl config view --raw --minify -o jsonpath='{.users[0].user.client-certificate-data}')
CURRENT_CLIENT_KEY=$(kubectl config view --raw --minify -o jsonpath='{.users[0].user.client-key-data}')

# Replace server with container-accessible endpoint
CONTAINER_SERVER=$(echo "$CURRENT_SERVER" | sed 's|127\.0\.0\.1|host.docker.internal|g' | sed 's|localhost|host.docker.internal|g' | sed 's|0\.0\.0\.0|host.docker.internal|g')

# Use template and substitute values
cp "$SCRIPT_DIR/../configs/kubeconfig-template" "$TEMP_KUBECONFIG"
sed -i.bak "s|https://host.docker.internal:6443|$CONTAINER_SERVER|g" "$TEMP_KUBECONFIG"
sed -i.bak "s|certificate-authority-data: \"\"|certificate-authority-data: $CURRENT_CERT_DATA|g" "$TEMP_KUBECONFIG"
sed -i.bak "s|client-certificate-data: \"\"|client-certificate-data: $CURRENT_CLIENT_CERT|g" "$TEMP_KUBECONFIG"
sed -i.bak "s|client-key-data: \"\"|client-key-data: $CURRENT_CLIENT_KEY|g" "$TEMP_KUBECONFIG"

# Start new container with kubeconfig mounted
docker run -d \
  --name radius-deployment-engine \
  -e RADIUSBACKENDURL=http://host.docker.internal:9000/apis/api.ucp.dev/v1alpha3 \
  -e KUBECONFIG=/root/.kube/config \
  -v "$TEMP_KUBECONFIG:/root/.kube/config:ro" \
  -p 5017:8080 \
  ghcr.io/radius-project/deployment-engine:latest

echo "✅ Deployment engine started on port 5017"

# Wait for health check
sleep 5
if curl -s http://localhost:5017/health >/dev/null; then
  echo "✅ Deployment engine health check passed"
else
  echo "⚠️  Deployment engine may still be starting up"
fi
