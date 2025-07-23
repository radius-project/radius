#!/bin/bash

# Get debug directory from environment or default
DEBUG_DEV_ROOT=${DEBUG_DEV_ROOT:-"$(pwd)/debug_files"}

cd "$DEBUG_DEV_ROOT"

echo "📊 Radius Component Status:"
echo "=========================="

components=("ucp" "controller" "applications-rp" "dynamic-rp")

for component in "${components[@]}"; do
  if [ -f "logs/${component}.pid" ]; then
    pid=$(cat "logs/${component}.pid")
    if kill -0 "$pid" 2>/dev/null; then
      echo "✅ $component (PID: $pid) - Running"
    else
      echo "❌ $component - PID file exists but process not running"
    fi
  else
    echo "❌ $component - Not running (no PID file)"
  fi
done

# Check deployment engine (Docker container)
echo ""
echo "🚢 Deployment Engine Status:"
echo "=========================="

if command -v docker >/dev/null 2>&1; then
  if docker ps --filter "name=radius-deployment-engine" --format "table {{.Names}}\t{{.Status}}" | grep -q radius-deployment-engine; then
    status=$(docker ps --filter "name=radius-deployment-engine" --format "{{.Status}}")
    echo "✅ deployment-engine (Docker) - Running ($status)"
  else
    echo "❌ deployment-engine - Not running (Docker container not found)"
  fi
else
  echo "⚠️  deployment-engine - Cannot check status (Docker not available)"
fi
