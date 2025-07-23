#!/bin/bash
set -e

echo "🚀 Starting deployment engine (Docker)..."

# Stop existing container if running
docker stop radius-deployment-engine 2>/dev/null || true
docker rm radius-deployment-engine 2>/dev/null || true

# Create kubeconfig for container access
echo "📝 Preparing kubeconfig for container..."
TEMP_KUBECONFIG="/tmp/radius-debug-kubeconfig"
kubectl config view --flatten --minify > "$TEMP_KUBECONFIG"

# Replace localhost/127.0.0.1 with host.docker.internal for container access
sed -i.bak 's|127\.0\.0\.1|host.docker.internal|g' "$TEMP_KUBECONFIG"
sed -i.bak 's|localhost|host.docker.internal|g' "$TEMP_KUBECONFIG"

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
