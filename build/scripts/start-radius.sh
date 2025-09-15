#!/bin/bash
set -e

# Get the script directory and repository root
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
DEBUG_ROOT="$REPO_ROOT/debug_files"

# PostgreSQL connection strings - try Docker first, fallback to local user
POSTGRES_ADMIN_CONNECTION="postgresql://postgres:radius_pass@localhost:5432/postgres"
POSTGRES_FALLBACK_CONNECTION="postgresql://$(whoami)@localhost:5432/postgres"

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Output helper functions (aligned with test.sh)
print_info() { echo -e "\033[0;34mℹ${NC} $1"; }
print_success() { echo -e "${GREEN}✓${NC} $1"; }
print_warning() { echo -e "${YELLOW}⚠${NC} $1"; }
print_error() { echo -e "${RED}✗${NC} $1"; }

# Helper function to execute PostgreSQL commands with proper connection
psql_exec() {
  local sql="$1"
  if psql "$POSTGRES_ADMIN_CONNECTION" -c "$sql" >/dev/null 2>&1; then
    return 0
  elif psql "$POSTGRES_FALLBACK_CONNECTION" -c "$sql" >/dev/null 2>&1; then
    return 0
  else
    return 1
  fi
}

check_prerequisites() {
  echo "🔍 Checking prerequisites (idempotent)..."
  local missing_tools=()
  local advisory_msgs=()

  # Required tools
  command -v dlv >/dev/null 2>&1 || missing_tools+=("dlv -> go install github.com/go-delve/delve/cmd/dlv@latest")
  command -v go >/dev/null 2>&1 || missing_tools+=("go -> https://golang.org/doc/install")
  command -v k3d >/dev/null 2>&1 || missing_tools+=("k3d -> https://k3d.io/")
  command -v kubectl >/dev/null 2>&1 || missing_tools+=("kubectl -> https://kubernetes.io/docs/tasks/tools/")
  command -v terraform >/dev/null 2>&1 || missing_tools+=("terraform -> https://developer.hashicorp.com/terraform/install")
  if ! command -v psql >/dev/null 2>&1; then
    missing_tools+=("psql (PostgreSQL client) -> https://www.postgresql.org/download/")
  else
    if psql "$POSTGRES_ADMIN_CONNECTION" -c "SELECT 1;" >/dev/null 2>&1; then
      print_info "PostgreSQL accessible via Docker (postgres user)"
    elif psql "$POSTGRES_FALLBACK_CONNECTION" -c "SELECT 1;" >/dev/null 2>&1; then
      print_info "PostgreSQL accessible via local user ($(whoami))"
    else
      advisory_msgs+=("PostgreSQL not reachable. Quick start: docker run --name radius-postgres -e POSTGRES_PASSWORD=radius_pass -p 5432:5432 -d postgres:15")
    fi
  fi

  if [ ${#missing_tools[@]} -ne 0 ]; then
    print_error "Missing required tools (install then re-run 'make debug-start'):";
    for tool in "${missing_tools[@]}"; do
      echo "  - $tool"
    done
    echo ""
    echo "Docs: docs/contributing/contributing-code/contributing-code-debugging/radius-os-processes-debugging.md#prerequisites"
    exit 1
  fi

  if [ ${#advisory_msgs[@]} -ne 0 ]; then
    print_warning "Advisories:";
    for msg in "${advisory_msgs[@]}"; do echo "  - $msg"; done
    echo "(Continuing; DB init will attempt creation)"
  fi

  print_success "Prerequisite check complete"
}

# Check if we have the debug environment set up
if [ ! -f "$DEBUG_ROOT/bin/ucpd" ]; then
  print_error "Debug environment not found. Please run 'make debug-setup' first."
  exit 1
fi

# Ensure logs directory exists
mkdir -p "$DEBUG_ROOT/logs"

# Check prerequisites
check_prerequisites

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

print_success "Cleanup complete"

# Ensure logs directory exists (double-check)
mkdir -p "$DEBUG_ROOT/logs"

# Initialize PostgreSQL database if needed
echo "🗄️  Initializing PostgreSQL database (idempotent)..."
if command -v psql >/dev/null 2>&1; then
  # First check if we can connect to PostgreSQL
  if ! psql_exec "SELECT 1;"; then
    print_error "Cannot connect to PostgreSQL"
    echo "Troubleshooting:"
    echo "  - macOS: brew services start postgresql"
    echo "  - Linux: sudo systemctl start postgresql"
    echo "  - Or run disposable container: docker run --name radius-postgres -e POSTGRES_PASSWORD=radius_pass -p 5432:5432 -d postgres:15"
    echo "Re-run: make debug-start"
    echo "Docs: docs/contributing/contributing-code/contributing-code-debugging/radius-os-processes-debugging.md#prerequisites"
    exit 1
  fi
  
  # Create applications_rp user if it doesn't exist
  if ! psql_exec "CREATE USER applications_rp WITH PASSWORD 'radius_pass';"; then
    echo "(applications_rp user exists)"
  else
    echo "Created user applications_rp"
  fi
  if ! psql_exec "CREATE DATABASE applications_rp;"; then
    echo "(applications_rp database exists)"
  else
    echo "Created database applications_rp"
  fi
  
  # Grant privileges
  psql_exec "GRANT ALL PRIVILEGES ON DATABASE applications_rp TO applications_rp;" || true
  
  # Create the resources table in applications_rp database
  if ! psql "postgresql://applications_rp:radius_pass@localhost:5432/applications_rp" -c "
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
  " 2>/dev/null; then
    print_warning "Could not verify/create applications_rp tables"
  fi
  
  # Also create UCP database for completeness
  if ! psql_exec "CREATE USER ucp WITH PASSWORD 'radius_pass';"; then
    echo "(ucp user exists)"
  else
    echo "Created user ucp"
  fi
  if ! psql_exec "CREATE DATABASE ucp;"; then
    echo "(ucp database exists)"
  else
    echo "Created database ucp"
  fi
  psql_exec "GRANT ALL PRIVILEGES ON DATABASE ucp TO ucp;" || true
  
  # Create the resources table in ucp database too
  if ! psql "postgresql://ucp:radius_pass@localhost:5432/ucp" -c "
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
  " 2>/dev/null; then
    print_warning "Could not verify/create UCP tables"
  fi
  
  print_success "Database initialization complete (idempotent)"
else
  print_error "psql not available - database cannot be initialized"
  exit 1
fi

# Start UCP with dlv
echo "Starting UCP with dlv on port 40001..."
dlv exec "$DEBUG_ROOT/bin/ucpd" --listen=127.0.0.1:40001 --headless=true --api-version=2 --accept-multiclient --continue -- --config-file="$SCRIPT_DIR/../configs/ucp.yaml" > "$DEBUG_ROOT/logs/ucp.log" 2>&1 &
echo $! > "$DEBUG_ROOT/logs/ucp.pid"

# Wait for UCP to start and complete initialization (this can take 60+ seconds)
echo "Waiting for UCP to initialize (this may take up to 2 minutes)..."
max_attempts=60
attempt=0
while [ $attempt -lt $max_attempts ]; do
  if curl -s "http://localhost:9000/apis/api.ucp.dev/v1alpha3" > /dev/null 2>&1; then
    # Check if initialization is complete by looking for the success message in logs
    if grep -q "Successfully registered manifests" "$DEBUG_ROOT/logs/ucp.log" 2>/dev/null; then
      break
    fi
  fi
  
  # Show progress every 10 seconds
  if [ $((attempt % 10)) -eq 0 ] && [ $attempt -gt 0 ]; then
    echo "  Still waiting for UCP initialization... (${attempt}s elapsed)"
  fi
  
  sleep 2
  attempt=$((attempt + 1))
done

# Verify UCP is fully ready
if [ $attempt -eq $max_attempts ]; then
  print_error "UCP failed to start within 2 minutes"
  echo "Check the UCP log for details: $DEBUG_ROOT/logs/ucp.log"
  exit 1
fi
print_success "UCP started and initialized successfully"

# Start Controller with dlv
echo "Starting Controller with dlv on port 40002..."
dlv exec "$DEBUG_ROOT/bin/controller" --listen=127.0.0.1:40002 --headless=true --api-version=2 --accept-multiclient --continue -- --config-file="$SCRIPT_DIR/../configs/controller.yaml" --cert-dir="" > "$DEBUG_ROOT/logs/controller.log" 2>&1 &
echo $! > "$DEBUG_ROOT/logs/controller.pid"

# Wait for Controller to start (check health endpoint)
echo "Waiting for Controller to start..."
attempt=0
max_attempts=15
while [ $attempt -lt $max_attempts ]; do
  if curl -s "http://localhost:7073/healthz" > /dev/null 2>&1; then
    break
  fi
  sleep 2
  attempt=$((attempt + 1))
done

if [ $attempt -eq $max_attempts ]; then
  print_warning "Controller health check failed, but continuing (check logs: $DEBUG_ROOT/logs/controller.log)"
else
  print_success "Controller started successfully"
fi

# Start Applications RP with dlv
echo "Starting Applications RP with dlv on port 40003..."
dlv exec "$DEBUG_ROOT/bin/applications-rp" --listen=127.0.0.1:40003 --headless=true --api-version=2 --accept-multiclient --continue -- --config-file="$SCRIPT_DIR/../configs/applications-rp.yaml" > "$DEBUG_ROOT/logs/applications-rp.log" 2>&1 &
echo $! > "$DEBUG_ROOT/logs/applications-rp.pid"

# Wait for Applications RP to start (it takes time to register with UCP)
echo "Waiting for Applications RP to start..."
attempt=0
max_attempts=15
while [ $attempt -lt $max_attempts ]; do
  if curl -s "http://localhost:8080/healthz" > /dev/null 2>&1; then
    break
  fi
  sleep 2
  attempt=$((attempt + 1))
done

if [ $attempt -eq $max_attempts ]; then
  print_warning "Applications RP health check failed, but continuing (check logs: $DEBUG_ROOT/logs/applications-rp.log)"
else
  print_success "Applications RP started successfully"
fi

# Start Dynamic RP with dlv
echo "Starting Dynamic RP with dlv on port 40004..."
dlv exec "$DEBUG_ROOT/bin/dynamic-rp" --listen=127.0.0.1:40004 --headless=true --api-version=2 --accept-multiclient --continue -- --config-file="$SCRIPT_DIR/../configs/dynamic-rp.yaml" > "$DEBUG_ROOT/logs/dynamic-rp.log" 2>&1 &
echo $! > "$DEBUG_ROOT/logs/dynamic-rp.pid"

# Wait for Dynamic RP to start
echo "Waiting for Dynamic RP to start..."
attempt=0
max_attempts=15
while [ $attempt -lt $max_attempts ]; do
  if curl -s "http://localhost:8082/healthz" > /dev/null 2>&1; then
    break
  fi
  sleep 2
  attempt=$((attempt + 1))
done

if [ $attempt -eq $max_attempts ]; then
  print_warning "Dynamic RP health check failed, but continuing (check logs: $DEBUG_ROOT/logs/dynamic-rp.log)"
else
  print_success "Dynamic RP started successfully"
fi

echo "🎉 All components started successfully with dlv debugging!"
echo "🔗 UCP API: http://localhost:9000 (dlv debug port 40001)"
echo "🔗 Applications RP: http://localhost:8080 (dlv debug port 40003)"
echo "🔗 Dynamic RP: http://localhost:8082 (dlv debug port 40004)"
echo "🔗 Controller Health: http://localhost:7073/healthz (dlv debug port 40002)"
echo "🐛 Attach VS Code debugger to dlv ports 40001-40004"
