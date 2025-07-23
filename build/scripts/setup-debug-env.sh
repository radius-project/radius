#!/bin/bash
# setup-debug-env.sh - Sets up Radius debug development environment
# Usage: setup-debug-env.sh <config-file> <dev-root>

set -euo pipefail

CONFIG_FILE="${1:-build/debug-config.yaml}"
DEV_ROOT="${2:-$HOME/radius-dev}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if required tools are available
check_prerequisites() {
    log_info "Checking prerequisites..."
    
    local missing_tools=()
    
    # Required tools
    command -v go >/dev/null 2>&1 || missing_tools+=("go")
    command -v psql >/dev/null 2>&1 || missing_tools+=("psql")
    command -v terraform >/dev/null 2>&1 || missing_tools+=("terraform")
    command -v rad >/dev/null 2>&1 || missing_tools+=("rad")
    command -v k3d >/dev/null 2>&1 || missing_tools+=("k3d")
    
    if [ ${#missing_tools[@]} -gt 0 ]; then
        log_error "Missing required tools: ${missing_tools[*]}"
        log_error "Please install the missing tools and run setup again"
        log_info "See docs/contributing/contributing-code/contributing-code-debugging/radius-os-processes-debugging.md for installation instructions"
        exit 1
    fi
    
    # Optional tools
    if ! command -v kubectl >/dev/null 2>&1; then
        log_warning "kubectl not found - will be provided by k3d"
    fi
    
    if ! command -v dotnet >/dev/null 2>&1; then
        log_warning "dotnet not found - local deployment engine debugging will not be available"
    fi
    
    if ! command -v docker >/dev/null 2>&1; then
        log_warning "docker not found - Docker-based deployment engine will not be available"
    fi
    
    log_success "All required prerequisites found"
}

# Create directory structure
create_directories() {
    log_info "Creating directory structure at $DEV_ROOT..."
    
    mkdir -p "$DEV_ROOT"/{configs,logs,bin,terraform-cache,terraform,data}
    
    log_success "Directory structure created"
}

# Generate configuration files
generate_configs() {
    log_info "Generating configuration files..."
    
    # UCP configuration with PostgreSQL (no Kubernetes required)
    cat > "$DEV_ROOT/configs/ucp.yaml" <<EOF
environment:
  name: "dev"
  roleLocation: "global"

server:
  port: 9000
  pathBase: /apis/api.ucp.dev/v1alpha3

databaseProvider:
  provider: "postgresql"
  postgresql:
    url: "postgresql://radius_user:radius_pass@localhost:5432/radius?sslmode=disable"

secretProvider:
  provider: "kubernetes"

queueProvider:
  provider: "inmemory"
  name: 'ucp'

profilerProvider:
  enabled: false
  port: 6061

initialization:
  planes:
    - id: "/planes/aws/aws"
      properties:
        kind: "AWS"
    - id: "/planes/radius/local"
      properties:
        resourceProviders:
          Applications.Core: "http://localhost:8080"
          Applications.Messaging: "http://localhost:8080"
          Applications.Dapr: "http://localhost:8080"
          Applications.Datastores: "http://localhost:8080"
          Microsoft.Resources: "http://localhost:5445"
        kind: "UCPNative"
    - id: "/planes/azure/azure"
      properties:
        kind: "Azure"
  manifestDirectory: "$(pwd)/deploy/manifest/built-in-providers/dev"

identity:
  authMethod: default

ucp:
  kind: direct
  direct:
    endpoint: "http://localhost:9000/apis/api.ucp.dev/v1alpha3"

routing:
  defaultDownstreamEndpoint: "http://localhost:8082"

metricsProvider:
  enabled: false
  serviceName: "ucp"
  prometheus:
    path: "/metrics"
    port: 9091

logging:
  level: "info"
  json: true

tracerProvider:
  enabled: false
  serviceName: "ucp"
  zipkin:
    url: "http://localhost:9411/api/v2/spans"
EOF

    # Applications RP configuration
    cat > "$DEV_ROOT/configs/applications-rp.yaml" <<EOF
environment:
  name: "dev"
  roleLocation: "global"
databaseProvider:
  provider: "postgresql"
  postgresql:
    url: "postgresql://radius_user:radius_pass@localhost:5432/radius?sslmode=disable"
queueProvider:
  provider: "inmemory"
  name: radius
secretProvider:
  provider: "kubernetes"
metricsProvider:
  enabled: false
  serviceName: applications-rp
  prometheus:
    path: "/metrics"
    port: 9092
profilerProvider:
  enabled: false
  port: 6060
featureFlags:
  - "PLACEHOLDER"
server:
  host: "0.0.0.0"
  port: 8080
  enableArmAuth: false
workerServer:
  maxOperationConcurrency: 10
  maxOperationRetryCount: 2
ucp:
  kind: direct
  direct:
    endpoint: "http://localhost:9000/apis/api.ucp.dev/v1alpha3"
logging:
  level: "info"
  json: false
tracerProvider:
  enabled: false
  serviceName: applications-rp
  zipkin:
    url: "http://localhost:9411/api/v2/spans"
bicep:
  deleteRetryCount: 20
  deleteRetryDelaySeconds: 60
terraform:
  path: "$DEV_ROOT/terraform"
EOF

    # Controller configuration
    cat > "$DEV_ROOT/configs/controller.yaml" <<EOF
environment:
  name: "dev"
  roleLocation: "global"
profilerProvider:
  enabled: false
  port: 6063
metricsProvider:
  enabled: false
  serviceName: "controller"
  prometheus:
    path: "/metrics"
    port: 9093
tracerProvider:
  enabled: false
  serviceName: "controller"
  zipkin:
    url: "http://localhost:9411/api/v2/spans"
server:
  host: "0.0.0.0"
  port: 8083
workerServer:
  port: 7073
ucp:
  kind: direct
  direct:
    endpoint: "http://localhost:9000/apis/api.ucp.dev/v1alpha3"
logging:
  level: "info"
  json: false
EOF

    # Dynamic RP configuration
    cat > "$DEV_ROOT/configs/dynamic-rp.yaml" <<EOF
environment:
  name: "dev"
  roleLocation: "global"
databaseProvider:
  provider: "postgresql"
  postgresql:
    url: "postgresql://radius_user:radius_pass@localhost:5432/radius?sslmode=disable"
queueProvider:
  provider: "inmemory"
  name: dynamic-rp
secretProvider:
  provider: "kubernetes"
profilerProvider:
  enabled: false
  port: 6062
metricsProvider:
  enabled: false
  serviceName: "dynamic-rp"
  prometheus:
    path: "/metrics"
    port: 9092
tracerProvider:
  enabled: false
  serviceName: "dynamic-rp"
  zipkin:
    url: "http://localhost:9411/api/v2/spans"
kubernetes:
  kind: default
server:
  host: "0.0.0.0"
  port: 8082
workerServer:
  maxOperationConcurrency: 10
  maxOperationRetryCount: 2
ucp:
  kind: direct
  direct:
    endpoint: "http://localhost:9000/apis/api.ucp.dev/v1alpha3"
logging:
  level: "info"
  json: false
bicep:
  deleteRetryCount: 20
  deleteRetryDelaySeconds: 60
terraform:
  path: "$DEV_ROOT/terraform"
EOF

    # Terraform CLI configuration
    cat > "$DEV_ROOT/configs/terraformrc" <<EOF
disable_checkpoint = true
plugin_cache_dir = "$DEV_ROOT/terraform-cache"

provider_installation {
  filesystem_mirror {
    path    = "$DEV_ROOT/terraform-cache/providers"
    include = ["registry.terraform.io/*/*"]
  }
  direct {
    exclude = ["registry.terraform.io/*/*"]
  }
}
EOF

    log_success "Configuration files generated"
}

# Generate environment setup script
generate_env_script() {
    log_info "Generating environment setup script..."
    
    cat > "$DEV_ROOT/env-setup.sh" <<EOF
#!/bin/bash
# Radius development environment setup
# Source this file to set up environment variables for debugging

# Core Radius Configuration
export RADIUS_ENV=self-hosted
export K8S_CLUSTER=true
export SKIP_ARM=false
export ARM_AUTH_METHOD=UCPCredential

# Development Paths
export RADIUS_DEV_ROOT="$DEV_ROOT"
export PATH="\$RADIUS_DEV_ROOT/bin:\$PATH"

# Database Configuration
export DATABASE_PROVIDER=postgresql
export DATABASE_CONNECTION_STRING="postgresql://radius_user:radius_pass@localhost:5432/radius"

# Kubernetes Configuration
export KUBECONFIG="$HOME/.kube/config"

# Terraform Configuration
export TF_CLI_CONFIG_FILE="\$RADIUS_DEV_ROOT/configs/terraformrc"

# UCP Configuration
export UCP_ENDPOINT=http://localhost:9000

# Service Endpoints
export APPLICATIONS_RP_ENDPOINT=http://localhost:5443
export DYNAMIC_RP_ENDPOINT=http://localhost:5445

# Deployment Engine Configuration (Docker by default)
export DEPLOYMENT_ENGINE_URL=http://localhost:5017
export DEPLOYMENT_ENGINE_ENDPOINT=http://localhost:5017

# Logging Configuration
export RADIUS_LOG_LEVEL=debug
export RADIUS_LOG_JSON=false

echo "✅ Radius development environment configured"
echo "📍 Dev root: \$RADIUS_DEV_ROOT"
echo "🔗 UCP endpoint: \$UCP_ENDPOINT"
EOF

    chmod +x "$DEV_ROOT/env-setup.sh"
    
    log_success "Environment setup script generated"
}

# Generate debug CLI configuration and wrapper
generate_debug_cli() {
    log_info "Generating debug CLI configuration and wrapper..."
    
    # Create debug CLI config that sets up workspace with UCP override (hardcoded for k3d)
    cat > "$DEV_ROOT/configs/rad-debug-config.yaml" <<EOF
workspaces:
  default: debug
  items:
    debug:
      connection:
        context: 'k3d-radius-debug'
        kind: kubernetes
        overrides:
          ucp: http://localhost:9000
      environment: /planes/radius/local/resourceGroups/default/providers/Applications.Core/environments/default
      scope: /planes/radius/local/resourceGroups/default
EOF

    # Create wrapper script that uses debug config (named 'rad-wrapper' for the symlink)
    cat > "$DEV_ROOT/bin/rad-wrapper" <<WRAPPER_EOF
#!/bin/bash
# Debug wrapper for rad CLI that automatically configures UCP endpoint

DEBUG_CONFIG="$DEV_ROOT/configs/rad-debug-config.yaml"
RAD_BINARY="$DEV_ROOT/bin/rad"

# Check if debug config exists
if [ ! -f "\$DEBUG_CONFIG" ]; then
    echo "Error: Debug config not found at \$DEBUG_CONFIG"
    echo "Run 'make debug-setup' to regenerate debug environment"
    exit 1
fi

# Check if rad binary exists
if [ ! -f "\$RAD_BINARY" ]; then
    echo "Error: rad binary not found at \$RAD_BINARY"
    echo "Run 'make debug-build-rad' to build the rad CLI"
    exit 1
fi

# Execute rad with debug config, passing through all arguments
exec "\$RAD_BINARY" --config "\$DEBUG_CONFIG" "\$@"
WRAPPER_EOF

    chmod +x "$DEV_ROOT/bin/rad-wrapper"
    
    log_success "Debug CLI wrapper generated at $DEV_ROOT/bin/rad-wrapper"
}

# Generate management scripts
generate_scripts() {
    log_info "Generating management scripts..."
    
    # Start script
    cat > "$DEV_ROOT/scripts/start-radius.sh" <<'EOF'
#!/bin/bash
set -e

cd "$(dirname "$0")/.."
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

# Kill any remaining Radius processes
pkill -f "ucpd" 2>/dev/null || true
pkill -f "applications-rp" 2>/dev/null || true
pkill -f "dynamic-rp" 2>/dev/null || true
pkill -f "controller.*--config-file.*controller.yaml" 2>/dev/null || true

echo "✅ Cleanup complete"

# Start UCP
echo "Starting UCP..."
./bin/ucpd --config-file=configs/ucp.yaml > logs/ucp.log 2>&1 &
echo $! > logs/ucp.pid
sleep 5

# Verify UCP
if ! curl -s "http://localhost:9000/apis/api.ucp.dev/v1alpha3" > /dev/null; then
  echo "❌ UCP failed to start"
  exit 1
fi
echo "✅ UCP started successfully"

# Start Controller
echo "Starting Controller..."
./bin/controller --config-file=configs/controller.yaml --cert-dir="" > logs/controller.log 2>&1 &
echo $! > logs/controller.pid
sleep 3

# Start Applications RP
echo "Starting Applications RP..."
./bin/applications-rp --config-file=configs/applications-rp.yaml > logs/applications-rp.log 2>&1 &
echo $! > logs/applications-rp.pid
sleep 3

# Start Dynamic RP
echo "Starting Dynamic RP..."
./bin/dynamic-rp --config-file=configs/dynamic-rp.yaml > logs/dynamic-rp.log 2>&1 &
echo $! > logs/dynamic-rp.pid
sleep 3

# Check deployment engine and start if needed
echo "Checking deployment engine..."
if command -v docker >/dev/null 2>&1; then
  if ! docker ps --filter "name=radius-deployment-engine" --format "{{.Names}}" | grep -q radius-deployment-engine; then
    echo "Starting deployment engine..."
    ./scripts/start-deployment-engine.sh
  else
    echo "✅ Deployment engine already running"
  fi
else
  echo "⚠️  Docker not available - deployment engine cannot be started"
fi

echo "🎉 All components started successfully!"
echo "🔗 UCP: http://localhost:9000"
echo "🔗 Applications RP: http://localhost:8080"
echo "🔗 Dynamic RP: http://localhost:8082"
echo "🔗 Controller Health: http://localhost:7073/healthz"
EOF

    # Stop script
    cat > "$DEV_ROOT/scripts/stop-radius.sh" <<'EOF'
#!/bin/bash

cd "$(dirname "$0")/.."

echo "🛑 Stopping Radius components..."

# Stop processes tracked by PID files
for component in dynamic-rp applications-rp controller ucp; do
  if [ -f "logs/${component}.pid" ]; then
    pid=$(cat "logs/${component}.pid")
    if kill -0 "$pid" 2>/dev/null; then
      echo "Stopping $component (PID: $pid)"
      kill "$pid"
      sleep 2
      if kill -0 "$pid" 2>/dev/null; then
        kill -9 "$pid"
      fi
    fi
    rm -f "logs/${component}.pid"
  fi
done

# Kill any remaining Radius processes that might not be tracked
echo "Cleaning up any remaining Radius processes..."
pkill -f "ucpd" && echo "Killed remaining ucpd processes" || true
pkill -f "applications-rp" && echo "Killed remaining applications-rp processes" || true
pkill -f "dynamic-rp" && echo "Killed remaining dynamic-rp processes" || true
pkill -f "controller.*--config-file.*controller.yaml" && echo "Killed remaining controller processes" || true

echo "✅ All components stopped"
EOF

    # Status script
    cat > "$DEV_ROOT/scripts/status-radius.sh" <<'EOF'
#!/bin/bash

cd "$(dirname "$0")/.."

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
EOF

    # Recipe registration script
    cat > "$DEV_ROOT/scripts/register-recipes.sh" <<'EOF'
#!/bin/bash
set -e

cd "$(dirname "$0")/.."

echo "🍴 Registering default recipes..."

# Check if rad wrapper exists
if [ ! -f bin/rad-wrapper ]; then
    echo "❌ rad-wrapper not found. Run setup first."
    exit 1
fi

# Wait for environment to be ready
echo "Waiting for environment to be available..."
max_attempts=30
attempt=0

while [ $attempt -lt $max_attempts ]; do
    if ./bin/rad-wrapper env show default >/dev/null 2>&1; then
        echo "✅ Environment 'default' is ready"
        break
    fi
    echo "Waiting for environment... (attempt $((attempt + 1))/$max_attempts)"
    sleep 2
    ((attempt++))
done

if [ $attempt -eq $max_attempts ]; then
    echo "❌ Environment not ready after ${max_attempts} attempts"
    echo "💡 Make sure to run: ./rad group create default && ./rad env create default"
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
    
    if ./bin/rad-wrapper recipe register "default" \
        --resource-type "$resource_type" \
        --template-kind "bicep" \
        --template-path "$template_path" \
        --environment default >/dev/null 2>&1; then
        echo "✅ Registered: default recipe for $resource_type"
        ((registered_count++))
    else
        echo "⚠️  Failed to register: default recipe for $resource_type"
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
echo "💡 All recipes are registered as 'default' so deployments will find them automatically"
EOF

    chmod +x "$DEV_ROOT/scripts"/*.sh
    
    log_success "Management scripts generated"
}



# Check if database is set up
check_database() {
    log_info "Checking database setup..."
    
    if ! psql "postgresql://radius_user:radius_pass@localhost:5432/radius" -c "SELECT 1;" >/dev/null 2>&1; then
        log_info "Database not found. Setting up PostgreSQL database..."
        
        # Check if PostgreSQL is running
        if ! pgrep postgres >/dev/null 2>&1; then
            log_error "PostgreSQL is not running. Please start PostgreSQL first:"
            echo "  brew services start postgresql"
            echo "  # or sudo systemctl start postgresql (Linux)"
            exit 1
        fi
        
        # Create database and user
        if sudo -u postgres psql 2>/dev/null <<EOF
CREATE DATABASE radius;
CREATE USER radius_user WITH PASSWORD 'radius_pass';
GRANT ALL PRIVILEGES ON DATABASE radius TO radius_user;
GRANT CREATE ON SCHEMA public TO radius_user;
EOF
        then
            log_success "Database and user created successfully"
            
            # Initialize database schema
            log_info "Initializing database schema..."
            if psql "postgresql://radius_user:radius_pass@localhost:5432/radius" < "$(pwd)/deploy/init-db/db.sql.txt" >/dev/null 2>&1; then
                log_success "Database schema initialized successfully"
            else
                log_error "Failed to initialize database schema"
                exit 1
            fi
        else
            # Try without sudo -u postgres (for Homebrew PostgreSQL)
            log_info "Trying alternative PostgreSQL setup method..."
            if psql postgres 2>/dev/null <<EOF
CREATE DATABASE radius;
CREATE USER radius_user WITH PASSWORD 'radius_pass';
GRANT ALL PRIVILEGES ON DATABASE radius TO radius_user;
GRANT CREATE ON SCHEMA public TO radius_user;
EOF
            then
                log_success "Database and user created successfully"
                
                # Initialize database schema
                log_info "Initializing database schema..."
                if psql "postgresql://radius_user:radius_pass@localhost:5432/radius" < "$(pwd)/deploy/init-db/db.sql.txt" >/dev/null 2>&1; then
                    log_success "Database schema initialized successfully"
                else
                    log_error "Failed to initialize database schema"
                    exit 1
                fi
            else
                log_error "Failed to create database automatically. Please run manually:"
                echo "  createdb radius"
                echo "  psql postgres -c \"CREATE USER radius_user WITH PASSWORD 'radius_pass';\""
                echo "  psql postgres -c \"GRANT ALL PRIVILEGES ON DATABASE radius TO radius_user;\""
                echo "  psql postgres -c \"GRANT CREATE ON SCHEMA public TO radius_user;\""
                exit 1
            fi
        fi
    else
        log_success "Database connection verified"
    fi
}

# Check Kubernetes setup
check_kubernetes() {
    log_info "Checking Kubernetes setup..."
    
    if ! kubectl cluster-info >/dev/null 2>&1; then
        log_error "Cannot connect to Kubernetes cluster"
        log_info "Please ensure kubectl is configured and you have access to a cluster"
        exit 1
    fi
    
    # Check if radius-system namespace exists
    if ! kubectl get namespace radius-system >/dev/null 2>&1; then
        log_info "Creating radius-system namespace..."
        kubectl create namespace radius-system
    fi
    
    # Check if radius-testing namespace exists
    if ! kubectl get namespace radius-testing >/dev/null 2>&1; then
        log_info "Creating radius-testing namespace..."
        kubectl create namespace radius-testing
    fi
    
    log_success "Kubernetes setup verified"
}

# Setup deployment engine
setup_deployment_engine() {
    log_info "Setting up deployment engine..."
    
    if command -v docker >/dev/null 2>&1; then
        log_info "Setting up Docker-based deployment engine..."
        cat > "$DEV_ROOT/scripts/start-deployment-engine.sh" <<'EOF'
#!/bin/bash
set -e

echo "🚀 Starting deployment engine (Docker)..."

# Stop existing container if running
docker stop radius-deployment-engine 2>/dev/null || true
docker rm radius-deployment-engine 2>/dev/null || true

# Start new container
docker run -d \
  --name radius-deployment-engine \
  -e RADIUSBACKENDURL=http://host.docker.internal:9000/apis/api.ucp.dev/v1alpha3 \
  -p 5017:8080 \
  ghcr.io/radius-project/deployment-engine:latest

echo "✅ Deployment engine started on port 5017"

# Wait for health check
sleep 5
if curl -s http://localhost:5017/health >/dev/null; then
  echo "✅ Deployment engine health check passed"
else
  echo "⚠️  Deployment engine may still be starting up"
fi
EOF

        cat > "$DEV_ROOT/scripts/stop-deployment-engine.sh" <<'EOF'
#!/bin/bash
echo "🛑 Stopping deployment engine..."
docker stop radius-deployment-engine 2>/dev/null || true
docker rm radius-deployment-engine 2>/dev/null || true
echo "✅ Deployment engine stopped"
EOF

        chmod +x "$DEV_ROOT/scripts/start-deployment-engine.sh"
        chmod +x "$DEV_ROOT/scripts/stop-deployment-engine.sh"
        
        log_success "Docker-based deployment engine scripts created"
    else
        log_warning "Docker not found - deployment engine will need to be set up manually"
    fi
}

# Setup Terraform symlinks for RPs
setup_terraform_symlinks() {
    log_info "Setting up Terraform binary for resource providers..."
    
    # Get the system terraform binary path
    local terraform_path
    terraform_path=$(command -v terraform)
    
    if [ -z "$terraform_path" ]; then
        log_error "Terraform binary not found in PATH"
        exit 1
    fi
    
    log_info "Found Terraform binary at: $terraform_path"
    
    # Both RPs now expect terraform binary at $DEV_ROOT/terraform/terraform
    local terraform_binary_path="$DEV_ROOT/terraform/terraform"
    
    # Create symlink to system terraform binary within debug environment
    if [ ! -e "$terraform_binary_path" ]; then
        ln -sf "$terraform_path" "$terraform_binary_path" && \
            log_success "Created Terraform symlink: $terraform_binary_path -> $terraform_path" || \
            log_warning "Could not create Terraform symlink"
    else
        log_info "Terraform symlink already exists: $terraform_binary_path"
    fi
    
    # Verify the symlink works
    if [ -x "$terraform_binary_path" ]; then
        log_success "Terraform binary is accessible to both Applications RP and Dynamic RP"
    else
        log_warning "Terraform binary may not be accessible to RPs"
    fi
}

# Main setup function
main() {
    log_info "Setting up Radius debug development environment..."
    log_info "Config file: $CONFIG_FILE"
    log_info "Development root: $DEV_ROOT"
    
    check_prerequisites
    create_directories
    generate_configs
    generate_env_script
    generate_debug_cli
    check_database
    # Skip Kubernetes check - k3d cluster will be created by debug-start
    # check_kubernetes
    setup_deployment_engine
    setup_terraform_symlinks
    
    log_success "Debug environment setup complete!"
    echo ""
    log_info "For more information, see:"
    echo "docs/contributing/contributing-code/contributing-code-debugging/radius-os-processes-debugging.md"
}

# Run main function
main "$@"
