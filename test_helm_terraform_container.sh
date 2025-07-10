#!/bin/bash
# Script to test Helm chart rendering with Terraform container feature

set -e

echo "Testing Helm chart rendering with Terraform container feature..."

# Test 1: Render chart with Terraform container enabled
echo "=== Test 1: Terraform container enabled ==="
helm template test-release deploy/Chart \
  --set global.terraform.enabled=true \
  --set global.terraform.image=ghcr.io/hashicorp/terraform \
  --set global.terraform.tag=latest \
  --debug > /tmp/terraform-enabled.yaml

# Check if init container is present
if grep -q "terraform-init" /tmp/terraform-enabled.yaml; then
  echo "✓ Init container found in rendered template"
else
  echo "✗ Init container NOT found in rendered template"
  exit 1
fi

# Check if the terraform binary copying logic is present
if grep -q "Copying terraform binary" /tmp/terraform-enabled.yaml; then
  echo "✓ Terraform binary copying logic found"
else
  echo "✗ Terraform binary copying logic NOT found"
  exit 1
fi

# Test 2: Render chart with Terraform container disabled (default)
echo "=== Test 2: Terraform container disabled (default) ==="
helm template test-release deploy/Chart \
  --debug > /tmp/terraform-disabled.yaml

# Check that init container is NOT present when disabled
if grep -q "terraform-init" /tmp/terraform-disabled.yaml; then
  echo "✗ Init container should NOT be present when feature is disabled"
  exit 1
else
  echo "✓ Init container correctly absent when feature is disabled"
fi

echo "✓ All Helm template tests passed!"

# Cleanup
rm -f /tmp/terraform-enabled.yaml /tmp/terraform-disabled.yaml
