#!/bin/bash
set -e

echo "==> Building Radius..."
make build && make generate && make test

echo "==> Setting Docker registry environment variables..."
export DOCKER_REGISTRY=ghcr.io/ytimocin
export DOCKER_TAG_VERSION=latest

echo "==> Building and pushing Docker images..."
make docker-build docker-push

echo "==> Resetting the Kind cluster..."
kind delete cluster && kind create cluster --wait 60s

echo "==> Installing Radius on Kubernetes..."

./dist/darwin_arm64/release/rad install kubernetes \
  --set rp.image=ghcr.io/ytimocin/applications-rp,rp.tag=latest \
  --set dynamicrp.image=ghcr.io/ytimocin/dynamic-rp,dynamicrp.tag=latest \
  --set controller.image=ghcr.io/ytimocin/controller,controller.tag=latest \
  --set ucp.image=ghcr.io/ytimocin/ucpd,ucp.tag=latest \
  --set bicep.image=ghcr.io/ytimocin/bicep,bicep.tag=latest \
  --reinstall

# ./dist/darwin_arm64/release/rad install kubernetes \
#   --set rp.image=ghcr.io/radius-project/dev/applications-rp,rp.tag=pr-func8b357be0dc \
#   --set dynamicrp.image=ghcr.io/radius-project/dev/dynamic-rp,dynamicrp.tag=pr-func8b357be0dc \
#   --set controller.image=ghcr.io/radius-project/dev/controller,controller.tag=pr-func8b357be0dc \
#   --set ucp.image=ghcr.io/radius-project/dev/ucpd,ucp.tag=pr-func8b357be0dc \
#   --set bicep.image=ghcr.io/radius-project/dev/bicep,bicep.tag=pr-func8b357be0dc

echo "==> Waiting for 30 seconds to allow services to initialize..."
sleep 30

echo "==> Create Radius workspace..."
./dist/darwin_arm64/release/rad workspace create kubernetes default --force

echo "==> Create Radius group..."
./dist/darwin_arm64/release/rad group create default

echo "==> Create Radius environment..."
./dist/darwin_arm64/release/rad environment create default

echo "==> Publishing Bicep extension..."
bicep publish-extension ./hack/bicep-types-radius/generated/index.json --target updated.zip

# echo "===> Deploy a Bicep file..."
# ./dist/darwin_arm64/release/rad deploy app-kubernetes-postgres.bicep \
#   --parameters username=postgres \
#   --parameters password=admin

# echo "==> Deployment complete!"
