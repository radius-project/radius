#!/bin/bash
set -e

# Check if we're in the right directory
if [ ! -f "debug_files/bin/ucpd" ]; then
    echo "❌ Please run this script from the repository root directory"
    exit 1
fi

cd debug_files
source env-setup.sh

echo "🚀 Starting Radius components..."

# Stop any existing components first
echo "Checking for existing components and stopping them..."
for component in dynamic-rp applications-rp controller ucp; do
  if [ -f "logs/${component}.pid" ]; then
    pid=$(cat "logs/${component}.pid")
    if kill -0 "$pid" 2>/dev/null; then
      echo "Stopping existing $component (PID: $pid)"
      kill "$pid" 2>/dev/null || true
      sleep 2
      if kill -0 "$pid" 2>/dev/null; then
        kill -9 "$pid" 2>/dev/null || true
      fi
    fi
    rm -f "logs/${component}.pid"
  fi
done

# Kill any remaining Radius processes and dlv
pkill -f "ucpd" 2>/dev/null || true
pkill -f "applications-rp" 2>/dev/null || true
pkill -f "dynamic-rp" 2>/dev/null || true
pkill -f "controller.*--config-file.*controller.yaml" 2>/dev/null || true
pkill -f "dlv.*exec.*ucpd" 2>/dev/null || true
pkill -f "dlv.*exec.*applications-rp" 2>/dev/null || true
pkill -f "dlv.*exec.*dynamic-rp" 2>/dev/null || true
pkill -f "dlv.*exec.*controller" 2>/dev/null || true

echo "✅ Cleanup complete"

# Start UCP with dlv
echo "Starting UCP with dlv on port 40001..."
dlv --listen=127.0.0.1:40001 --headless=true --api-version=2 --accept-multiclient --continue exec ./bin/ucpd -- --config-file=configs/ucp.yaml > logs/ucp.log 2>&1 &
echo $! > logs/ucp.pid
sleep 5

# Verify UCP
if ! curl -s "http://localhost:9000/apis/api.ucp.dev/v1alpha3" > /dev/null; then
  echo "❌ UCP failed to start"
  exit 1
fi
echo "✅ UCP started successfully"

# Start Controller with dlv
echo "Starting Controller with dlv on port 40002..."
dlv --listen=127.0.0.1:40002 --headless=true --api-version=2 --accept-multiclient --continue exec ./bin/controller -- --config-file=configs/controller.yaml --cert-dir="" > logs/controller.log 2>&1 &
echo $! > logs/controller.pid
sleep 3

# Start Applications RP with dlv
echo "Starting Applications RP with dlv on port 40003..."
dlv --listen=127.0.0.1:40003 --headless=true --api-version=2 --accept-multiclient --continue exec ./bin/applications-rp -- --config-file=configs/applications-rp.yaml > logs/applications-rp.log 2>&1 &
echo $! > logs/applications-rp.pid
sleep 3

# Start Dynamic RP with dlv
echo "Starting Dynamic RP with dlv on port 40004..."
dlv --listen=127.0.0.1:40004 --headless=true --api-version=2 --accept-multiclient --continue exec ./bin/dynamic-rp -- --config-file=configs/dynamic-rp.yaml > logs/dynamic-rp.log 2>&1 &
echo $! > logs/dynamic-rp.pid
sleep 3

# Check deployment engine and start if needed
echo "Checking deployment engine..."
if command -v docker >/dev/null 2>&1; then
  if ! docker ps --filter "name=radius-deployment-engine" --format "{{.Names}}" | grep -q radius-deployment-engine; then
    echo "Starting deployment engine..."
    if [ -f "scripts/start-deployment-engine.sh" ]; then
      ./scripts/start-deployment-engine.sh
    else
      echo "⚠️  Deployment engine start script not found"
    fi
  else
    echo "✅ Deployment engine already running"
  fi
else
  echo "⚠️  Docker not available - deployment engine cannot be started"
fi

echo "🎉 All components started successfully with dlv debugging!"
echo "🔗 UCP API: http://localhost:9000 (dlv debug port 40001)"
echo "🔗 Applications RP: http://localhost:8080 (dlv debug port 40003)"
echo "🔗 Dynamic RP: http://localhost:8082 (dlv debug port 40004)"
echo "🔗 Controller Health: http://localhost:7073/healthz (dlv debug port 40002)"
echo "🐛 Attach VS Code debugger to dlv ports 40001-40004"
