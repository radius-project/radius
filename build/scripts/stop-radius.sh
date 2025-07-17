#!/bin/bash
set -e

# Check if we're in the right directory
if [ ! -d "debug_files" ]; then
    echo "❌ Please run this script from the repository root directory"
    exit 1
fi

cd debug_files

echo "🛑 Stopping Radius components..."

# Stop components by PID if available
for component in dynamic-rp applications-rp controller ucp; do
  if [ -f "logs/${component}.pid" ]; then
    pid=$(cat "logs/${component}.pid")
    if kill -0 "$pid" 2>/dev/null; then
      echo "Stopping $component (PID: $pid)"
      kill "$pid" 2>/dev/null || true
      sleep 2
      # Force kill if still running
      if kill -0 "$pid" 2>/dev/null; then
        echo "Force stopping $component"
        kill -9 "$pid" 2>/dev/null || true
      fi
    fi
    rm -f "logs/${component}.pid"
  fi
done

# Kill any remaining Radius processes and dlv
echo "Cleaning up any remaining processes..."
pkill -f "ucpd" 2>/dev/null || true
pkill -f "applications-rp" 2>/dev/null || true
pkill -f "dynamic-rp" 2>/dev/null || true
pkill -f "controller.*--config-file.*controller.yaml" 2>/dev/null || true
pkill -f "dlv.*exec.*ucpd" 2>/dev/null || true
pkill -f "dlv.*exec.*applications-rp" 2>/dev/null || true
pkill -f "dlv.*exec.*dynamic-rp" 2>/dev/null || true
pkill -f "dlv.*exec.*controller" 2>/dev/null || true

# Stop deployment engine if running
echo "Stopping deployment engine..."
if command -v docker >/dev/null 2>&1; then
  if docker ps --filter "name=radius-deployment-engine" --format "{{.Names}}" | grep -q radius-deployment-engine; then
    docker stop radius-deployment-engine 2>/dev/null || true
    docker rm radius-deployment-engine 2>/dev/null || true
    echo "✅ Deployment engine stopped"
  else
    echo "✅ Deployment engine was not running"
  fi
else
  echo "⚠️  Docker not available - skipping deployment engine cleanup"
fi

echo "✅ All Radius components stopped"
