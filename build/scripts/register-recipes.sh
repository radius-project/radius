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

# Register default recipes for common resource types by updating the environment directly.
# Each recipe is registered with the name "default" so deployments can find them automatically.
ENVIRONMENT_NAMESPACE="${ENVIRONMENT_NAMESPACE:-default}"
recipes=(
    "Applications.Datastores/redisCaches:ghcr.io/radius-project/recipes/local-dev/rediscaches:latest"
    "Applications.Datastores/sqlDatabases:ghcr.io/radius-project/recipes/local-dev/sqldatabases:latest"
    "Applications.Datastores/mongoDatabases:ghcr.io/radius-project/recipes/local-dev/mongodatabases:latest"
    "Applications.Messaging/rabbitMQQueues:ghcr.io/radius-project/recipes/local-dev/rabbitmqqueues:latest"
)

recipes_json=""
for recipe_spec in "${recipes[@]}"; do
    # Split resource_type:template_path
    IFS=':' read -r resource_type template_path <<< "$recipe_spec"
    echo "Adding default recipe for $resource_type -> $template_path"
    recipes_json+="${recipes_json:+,}\"$resource_type\":{\"default\":{\"templateKind\":\"bicep\",\"templatePath\":\"$template_path\"}}"
done

env_file="$(mktemp)"
trap 'rm -f "$env_file"' EXIT

cat > "$env_file" <<EOF
{
  "location": "global",
  "properties": {
    "compute": {
      "kind": "kubernetes",
      "namespace": "$ENVIRONMENT_NAMESPACE"
    },
    "recipes": {${recipes_json}}
  }
}
EOF

if ! "$RAD_WRAPPER" resource create "Applications.Core/environments" "default" --from-file "$env_file"; then
    echo "❌ Failed to register default recipes on environment 'default'"
    exit 1
fi

echo ""
echo "🎉 Recipe registration complete!"
echo "💡 You can now deploy applications that use these resource types"
echo "📋 All recipes are registered as 'default' so deployments will find them automatically"
