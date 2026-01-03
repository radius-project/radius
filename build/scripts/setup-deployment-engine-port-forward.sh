#!/bin/bash

# Get the project root directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
DEBUG_ROOT="$REPO_ROOT/debug_files"

# Ensure logs directory exists
mkdir -p "$DEBUG_ROOT/logs"

echo "Setting up port forwarding for deployment engine..."

# Kill any existing port forward
pkill -f "port-forward.*deployment-engine" 2>/dev/null || true

# Start port forwarding in background and save PID
kubectl --context k3d-radius-debug port-forward -n default service/deployment-engine 5017:5445 > "$DEBUG_ROOT/logs/de-port-forward.log" 2>&1 &
port_forward_pid=$!

# Save the PID for later cleanup
echo $port_forward_pid > "$DEBUG_ROOT/logs/de-port-forward.pid"

echo "Port forwarding started (PID: $port_forward_pid)"

# Wait for deployment engine health check
echo "Waiting for deployment engine health check..."
max_attempts=30
attempt=0
while [ $attempt -lt $max_attempts ]; do
    if curl -s "http://localhost:5017/metrics" > /dev/null 2>&1; then
        echo "✅ Deployment Engine is ready"
        exit 0
    fi
    echo "Waiting for Deployment Engine... (attempt $((attempt + 1))/$max_attempts)"
    sleep 2
    attempt=$((attempt + 1))
done

echo "❌ Deployment Engine not ready after $max_attempts attempts"
echo "💡 Check component logs with 'make debug-logs' and 'make debug-deployment-engine-logs'"
exit 1