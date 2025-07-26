#!/bin/bash
set -e

# Get the script directory and repository root
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
DEBUG_ROOT="$REPO_ROOT/debug_files"

# Check if we have the debug environment set up
if [ ! -f "$DEBUG_ROOT/bin/ucpd" ]; then
    echo "❌ Debug environment not found. Please run 'make debug-setup' first."
    exit 1
fi

echo "🚀 Starting Radius components..."

# Stop any existing components first
echo "Checking for existing components and stopping them..."
for component in dynamic-rp applications-rp controller ucp; do
  if [ -f "$DEBUG_ROOT/logs/${component}.pid" ]; then
    pid=$(cat "$DEBUG_ROOT/logs/${component}.pid")
    if kill -0 "$pid" 2>/dev/null; then
      echo "Stopping existing $component (PID: $pid)"
      kill "$pid" 2>/dev/null || true
      sleep 2
      if kill -0 "$pid" 2>/dev/null; then
        kill -9 "$pid" 2>/dev/null || true
      fi
    fi
    rm -f "$DEBUG_ROOT/logs/${component}.pid"
  fi
done

# Kill any remaining Radius processes and dlv
# Use portable process killing that works on both macOS and Linux
if command -v pgrep >/dev/null 2>&1; then
  # Use pgrep/pkill if available (most Linux/macOS systems)
  pkill -f "ucpd" 2>/dev/null || true
  pkill -f "applications-rp" 2>/dev/null || true
  pkill -f "dynamic-rp" 2>/dev/null || true
  pkill -f "controller.*--config-file.*controller.yaml" 2>/dev/null || true
  pkill -f "dlv.*exec.*ucpd" 2>/dev/null || true
  pkill -f "dlv.*exec.*applications-rp" 2>/dev/null || true
  pkill -f "dlv.*exec.*dynamic-rp" 2>/dev/null || true
  pkill -f "dlv.*exec.*controller" 2>/dev/null || true
else
  # Fallback for systems without pkill
  ps aux | grep -E "(ucpd|applications-rp|dynamic-rp|controller.*--config-file.*controller.yaml|dlv.*exec)" | grep -v grep | awk '{print $2}' | xargs -r kill 2>/dev/null || true
fi

echo "✅ Cleanup complete"

# Initialize PostgreSQL database if needed
echo "🗄️  Initializing PostgreSQL database..."
if command -v psql >/dev/null 2>&1; then
  # Create applications_rp user if it doesn't exist
  psql "postgresql://$(whoami)@localhost:5432/postgres" -c "CREATE USER applications_rp WITH PASSWORD 'radius_pass';" 2>/dev/null || echo "User applications_rp already exists"
  
  # Create applications_rp database if it doesn't exist
  psql "postgresql://$(whoami)@localhost:5432/postgres" -c "CREATE DATABASE applications_rp;" 2>/dev/null || echo "Database applications_rp already exists"
  
  # Grant privileges
  psql "postgresql://$(whoami)@localhost:5432/postgres" -c "GRANT ALL PRIVILEGES ON DATABASE applications_rp TO applications_rp;" 2>/dev/null || true
  
  # Create the resources table in applications_rp database
  psql "postgresql://applications_rp:radius_pass@localhost:5432/applications_rp" -c "
  CREATE TABLE IF NOT EXISTS resources (
    id TEXT PRIMARY KEY NOT NULL,
    original_id TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    root_scope TEXT NOT NULL,
    routing_scope TEXT NOT NULL,
    etag TEXT NOT NULL,
    created_at timestamp(6) with time zone DEFAULT CURRENT_TIMESTAMP,
    resource_data jsonb NOT NULL
  );
  CREATE INDEX IF NOT EXISTS idx_resource_query ON resources (resource_type, root_scope);
  " 2>/dev/null || echo "Resources table setup completed"
  
  # Also create UCP database for completeness
  psql "postgresql://$(whoami)@localhost:5432/postgres" -c "CREATE USER ucp WITH PASSWORD 'radius_pass';" 2>/dev/null || echo "User ucp already exists"
  psql "postgresql://$(whoami)@localhost:5432/postgres" -c "CREATE DATABASE ucp;" 2>/dev/null || echo "Database ucp already exists"
  psql "postgresql://$(whoami)@localhost:5432/postgres" -c "GRANT ALL PRIVILEGES ON DATABASE ucp TO ucp;" 2>/dev/null || true
  
  # Create the resources table in ucp database too
  psql "postgresql://ucp:radius_pass@localhost:5432/ucp" -c "
  CREATE TABLE IF NOT EXISTS resources (
    id TEXT PRIMARY KEY NOT NULL,
    original_id TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    root_scope TEXT NOT NULL,
    routing_scope TEXT NOT NULL,
    etag TEXT NOT NULL,
    created_at timestamp(6) with time zone DEFAULT CURRENT_TIMESTAMP,
    resource_data jsonb NOT NULL
  );
  CREATE INDEX IF NOT EXISTS idx_resource_query ON resources (resource_type, root_scope);
  " 2>/dev/null || echo "UCP resources table setup completed"
  
  echo "✅ Database initialization complete"
else
  echo "⚠️  psql not available - database may not be properly initialized"
fi

# Start UCP with dlv
echo "Starting UCP with dlv on port 40001..."
dlv exec "$DEBUG_ROOT/bin/ucpd" --listen=127.0.0.1:40001 --headless=true --api-version=2 --accept-multiclient --continue -- --config-file="$SCRIPT_DIR/../configs/ucp.yaml" > "$DEBUG_ROOT/logs/ucp.log" 2>&1 &
echo $! > "$DEBUG_ROOT/logs/ucp.pid"
sleep 5

# Verify UCP
if ! curl -s "http://localhost:9000/apis/api.ucp.dev/v1alpha3" > /dev/null; then
  echo "❌ UCP failed to start"
  exit 1
fi
echo "✅ UCP started successfully"

# Start Controller with dlv
echo "Starting Controller with dlv on port 40002..."
dlv exec "$DEBUG_ROOT/bin/controller" --listen=127.0.0.1:40002 --headless=true --api-version=2 --accept-multiclient --continue -- --config-file="$SCRIPT_DIR/../configs/controller.yaml" --cert-dir="" > "$DEBUG_ROOT/logs/controller.log" 2>&1 &
echo $! > "$DEBUG_ROOT/logs/controller.pid"
sleep 3

# Start Applications RP with dlv
echo "Starting Applications RP with dlv on port 40003..."
dlv exec "$DEBUG_ROOT/bin/applications-rp" --listen=127.0.0.1:40003 --headless=true --api-version=2 --accept-multiclient --continue -- --config-file="$SCRIPT_DIR/../configs/applications-rp.yaml" > "$DEBUG_ROOT/logs/applications-rp.log" 2>&1 &
echo $! > "$DEBUG_ROOT/logs/applications-rp.pid"
sleep 3

# Start Dynamic RP with dlv
echo "Starting Dynamic RP with dlv on port 40004..."
dlv exec "$DEBUG_ROOT/bin/dynamic-rp" --listen=127.0.0.1:40004 --headless=true --api-version=2 --accept-multiclient --continue -- --config-file="$SCRIPT_DIR/../configs/dynamic-rp.yaml" > "$DEBUG_ROOT/logs/dynamic-rp.log" 2>&1 &
echo $! > "$DEBUG_ROOT/logs/dynamic-rp.pid"
sleep 3

echo "🎉 All components started successfully with dlv debugging!"
echo "🔗 UCP API: http://localhost:9000 (dlv debug port 40001)"
echo "🔗 Applications RP: http://localhost:8080 (dlv debug port 40003)"
echo "🔗 Dynamic RP: http://localhost:8082 (dlv debug port 40004)"
echo "🔗 Controller Health: http://localhost:7073/healthz (dlv debug port 40002)"
echo "🐛 Attach VS Code debugger to dlv ports 40001-40004"
