#!/bin/bash

# Get the project root directory (where this script is called from)
PROJECT_ROOT="$(pwd)"
RAD_WRAPPER="$PROJECT_ROOT/build/scripts/rad-wrapper"

echo "📝 Registering default recipes..."

# Check if rad-wrapper exists
if [ ! -f "$RAD_WRAPPER" ]; then
    echo "❌ rad-wrapper script not found at $RAD_WRAPPER"
    exit 1
fi

# Wait for environment to be ready
echo "Waiting for environment to be available..."
max_attempts=30
attempt=0

while [ $attempt -lt $max_attempts ]; do
    if "$RAD_WRAPPER" env show default >/dev/null 2>&1; then
        echo "✅ Environment 'default' is ready"
        break
    fi
    echo "Waiting for environment... (attempt $((attempt + 1))/$max_attempts)"
    sleep 2
    ((attempt++))
done

if [ $attempt -eq $max_attempts ]; then
    echo "❌ Environment not ready after ${max_attempts} attempts"
    echo "💡 Make sure to run: build/scripts/rad-wrapper group create default && build/scripts/rad-wrapper env create default"
    exit 1
fi

# Register default recipes for common resource types
# Each recipe is registered with the name "default" so deployments can find them automatically
recipes=(
    "Applications.Datastores/redisCaches:ghcr.io/radius-project/recipes/local-dev/rediscaches:latest"
    "Applications.Datastores/sqlDatabases:ghcr.io/radius-project/recipes/local-dev/sqldatabases:latest"
    "Applications.Datastores/mongoDatabases:ghcr.io/radius-project/recipes/local-dev/mongodatabases:latest"
    "Applications.Messaging/rabbitMQQueues:ghcr.io/radius-project/recipes/local-dev/rabbitmqqueues:latest"
)

registered_count=0
failed_count=0

for recipe_spec in "${recipes[@]}"; do
    # Split resource_type:template_path
    IFS=':' read -r resource_type template_path <<< "$recipe_spec"

    echo "Registering default recipe for $resource_type -> $template_path"

    # Try to register the recipe
    if output=$("$RAD_WRAPPER" recipe register "default" \
        --resource-type "$resource_type" \
        --template-kind "bicep" \
        --template-path "$template_path" \
        --environment default 2>&1); then
        echo "✅ Registered: default recipe for $resource_type"
        ((registered_count++))
    elif echo "$output" | grep -q "already exists\|already registered"; then
        echo "ℹ️  Already exists: default recipe for $resource_type"
        ((registered_count++))
    else
        echo "⚠️  Failed to register: default recipe for $resource_type"
        echo "   Error: $output"
        ((failed_count++))
    fi
done

echo ""
echo "📊 Recipe Registration Summary:"
echo "✅ Successfully registered: $registered_count"
if [ $failed_count -gt 0 ]; then
    echo "⚠️  Failed to register: $failed_count"
fi

echo ""
echo "🎉 Recipe registration complete!"
echo "💡 You can now deploy applications that use these resource types"
echo "📋 All recipes are registered as 'default' so deployments will find them automatically"

# Explicitly exit with success
exit 0
