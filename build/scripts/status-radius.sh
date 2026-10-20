#!/bin/bash

# Get debug directory from environment or default
DEBUG_DEV_ROOT=${DEBUG_DEV_ROOT:-"$(pwd)/debug_files"}

cd "$DEBUG_DEV_ROOT" || exit 1

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

# Check deployment engine (k3d deployment)
echo ""
echo "🚢 Deployment Engine Status:"
echo "=========================="

if command -v kubectl >/dev/null 2>&1; then
  if kubectl --context k3d-radius-debug get deployment deployment-engine -n default >/dev/null 2>&1; then
    status=$(kubectl --context k3d-radius-debug get deployment deployment-engine -n default -o jsonpath='{.status.conditions[?(@.type=="Available")].status}' 2>/dev/null)
    if [ "$status" = "True" ]; then
      replicas=$(kubectl --context k3d-radius-debug get deployment deployment-engine -n default -o jsonpath='{.status.readyReplicas}' 2>/dev/null)
      echo "✅ deployment-engine (k3d) - Running ($replicas replicas ready)"
    else
      echo "❌ deployment-engine (k3d) - Not ready"
    fi
  else
    echo "❌ deployment-engine - Not found in k3d cluster"
  fi
else
  echo "⚠️  deployment-engine - Cannot check status (kubectl not available)"
fi
