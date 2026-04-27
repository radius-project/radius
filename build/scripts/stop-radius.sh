#!/bin/bash
set -e

# PostgreSQL connection strings - try Docker first, fallback to local user
POSTGRES_ADMIN_CONNECTION="postgresql://postgres:radius_pass@localhost:5432/postgres"
POSTGRES_FALLBACK_CONNECTION="postgresql://$(whoami)@localhost:5432/postgres"
POSTGRES_CONTAINER_NAME="radius-postgres"

# Helper function to execute PostgreSQL commands with proper connection
# Tries: 1) local psql with Docker connection, 2) local psql with fallback connection (current user on localhost), 3) docker exec
psql_exec() {
  local sql="$1"
  if command -v psql >/dev/null 2>&1; then
    if psql "$POSTGRES_ADMIN_CONNECTION" -c "$sql" >/dev/null 2>&1; then
      return 0
    elif psql "$POSTGRES_FALLBACK_CONNECTION" -c "$sql" >/dev/null 2>&1; then
      return 0
    fi
  fi
  if docker exec "$POSTGRES_CONTAINER_NAME" psql -U postgres -c "$sql" >/dev/null 2>&1; then
    return 0
  fi
  return 1
}

echo "🧹 Full environment cleanup: stopping debug processes, deleting k3d cluster, deleting PostgreSQL data and schema..."

cd debug_files 2>/dev/null || {
  echo "⚠️  debug_files directory not found, stopping processes anyway..."
}

# Stop components using PID files if available
for component in dynamic-rp applications-rp controller ucp; do
  if [ -f "logs/${component}.pid" ]; then
    pid=$(cat "logs/${component}.pid")
    if kill -0 "$pid" 2>/dev/null; then
      echo "Stopping $component (PID: $pid)"
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
if command -v pgrep >/dev/null 2>&1; then
  pkill -f "ucpd" 2>/dev/null || true
  pkill -f "applications-rp" 2>/dev/null || true
  pkill -f "dynamic-rp" 2>/dev/null || true
  pkill -f "controller.*--config-file.*controller.yaml" 2>/dev/null || true
  pkill -f "dlv.*exec.*ucpd" 2>/dev/null || true
  pkill -f "dlv.*exec.*applications-rp" 2>/dev/null || true
  pkill -f "dlv.*exec.*dynamic-rp" 2>/dev/null || true
  pkill -f "dlv.*exec.*controller" 2>/dev/null || true
else
  ps aux | grep -E "(ucpd|applications-rp|dynamic-rp|controller.*--config-file.*controller.yaml|dlv.*exec)" | grep -v grep | awk '{print $2}' | xargs -r kill 2>/dev/null || true
fi

# Stop deployment engine in k3d cluster
echo "Stopping deployment engine..."
if command -v kubectl >/dev/null 2>&1; then
  kubectl --context k3d-radius-debug delete deployment deployment-engine 2>/dev/null || true
  kubectl --context k3d-radius-debug delete service deployment-engine 2>/dev/null || true
else
  echo "⚠️  kubectl not available - skipping deployment engine cleanup"
fi

# Nuclear database cleanup - drop everything
echo "💣 Nuclear database cleanup..."
if command -v psql >/dev/null 2>&1; then
  # Truncate tables first (if they exist)
  psql "postgresql://applications_rp:radius_pass@localhost:5432/applications_rp" -c "TRUNCATE TABLE resources;" 2>/dev/null || true
  psql "postgresql://ucp:radius_pass@localhost:5432/ucp" -c "TRUNCATE TABLE resources;" 2>/dev/null || true
  
  # Drop databases and users
  psql_exec "DROP DATABASE IF EXISTS applications_rp;" || true
  psql_exec "DROP DATABASE IF EXISTS ucp;" || true
  psql_exec "DROP DATABASE IF EXISTS radius;" || true
  psql_exec "DROP USER IF EXISTS applications_rp;" || true
  psql_exec "DROP USER IF EXISTS ucp;" || true
  psql_exec "DROP USER IF EXISTS radius;" || true
  
  echo "✅ Database nuclear cleanup complete"
elif docker exec "$POSTGRES_CONTAINER_NAME" psql -U postgres -c "SELECT 1;" >/dev/null 2>&1; then
  # Fall back to docker exec for truncation (needs specific database target)
  docker exec "$POSTGRES_CONTAINER_NAME" psql -U postgres -d applications_rp -c "TRUNCATE TABLE resources;" 2>/dev/null || true
  docker exec "$POSTGRES_CONTAINER_NAME" psql -U postgres -d ucp -c "TRUNCATE TABLE resources;" 2>/dev/null || true
  
  # Drop databases and users via psql_exec (handles docker exec fallback)
  psql_exec "DROP DATABASE IF EXISTS applications_rp;" || true
  psql_exec "DROP DATABASE IF EXISTS ucp;" || true
  psql_exec "DROP DATABASE IF EXISTS radius;" || true
  psql_exec "DROP USER IF EXISTS applications_rp;" || true
  psql_exec "DROP USER IF EXISTS ucp;" || true
  psql_exec "DROP USER IF EXISTS radius;" || true
  
  echo "✅ Database nuclear cleanup complete (via Docker)"
else
  echo "⚠️  Neither psql nor Docker container available - skipping database cleanup"
fi

# Nuclear k3d cleanup
echo "💣 Nuclear k3d cleanup..."
if command -v k3d >/dev/null 2>&1; then
  k3d cluster delete radius-debug 2>/dev/null || true
  echo "✅ k3d cluster destroyed"
else
  echo "⚠️  k3d not available - skipping cluster cleanup"
fi

echo "💥 Nuclear stop complete - everything destroyed!"
