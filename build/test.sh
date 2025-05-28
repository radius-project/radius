#!/bin/bash

# ------------------------------------------------------------
# Copyright 2023 The Radius Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#    
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
# ------------------------------------------------------------

set -euo pipefail

# Colors for output
readonly RED='\033[0;31m'
readonly GREEN='\033[0;32m'
readonly YELLOW='\033[1;33m'
readonly BLUE='\033[0;34m'
readonly NC='\033[0m' # No Color

# Function to print colored output
print_info() {
    echo -e "${BLUE}ℹ${NC} $1"
}

print_success() {
    echo -e "${GREEN}✓${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}⚠${NC} $1"
}

print_error() {
    echo -e "${RED}✗${NC} $1"
}

# Check if Azure CLI is installed and authenticated
check_azure_cli() {
    if ! command -v az &> /dev/null; then
        print_error "Azure CLI (az) is required but not installed."
        exit 1
    fi

    # Check if user is authenticated
    if ! az account show &> /dev/null; then
        print_error "Azure CLI is not authenticated."
        print_info "Please run 'az login' to authenticate."
        exit 1
    fi
    
    print_success "Azure CLI is installed and authenticated."
}

# Deploy long-running test cluster
deploy_lrt_cluster() {
    print_info "Deploying long-running test cluster to Azure..."
    
    # Default values (can be overridden by environment variables)
    local location="${AZURE_LOCATION:-westus3}"
    local resource_group="${LRT_RG:-$(whoami)-radius-lrt}"
    local grafana_enabled="${GRAFANA_ENABLED:-false}"
    local grafana_admin_object_id="${GRAFANA_ADMIN_OBJECT_ID:-}"
    
    print_info "Configuration:"
    print_info "  Location: $location"
    print_info "  Resource Group: $resource_group"
    print_info "  Grafana Enabled: $grafana_enabled"
    
    # Check if we need Grafana admin object ID
    if [[ "$grafana_enabled" == "true" && -z "$grafana_admin_object_id" ]]; then
        print_warning "Grafana is enabled but GRAFANA_ADMIN_OBJECT_ID is not set."
        print_info "Getting current user object ID..."
        grafana_admin_object_id=$(az ad signed-in-user show --query objectId -o tsv)
        print_info "Using current user object ID: $grafana_admin_object_id"
    fi
    
    # Step 1: Check and register feature flag
    print_info "Checking Microsoft.ContainerService/EnableImageCleanerPreview feature flag..."
    local feature_state
    feature_state=$(az feature show --namespace "Microsoft.ContainerService" --name "EnableImageCleanerPreview" --query properties.state -o tsv 2>/dev/null || echo "NotRegistered")
    
    if [[ "$feature_state" != "Registered" ]]; then
        print_warning "Feature flag is not registered. Registering now..."
        az feature register --namespace "Microsoft.ContainerService" --name "EnableImageCleanerPreview"
        print_info "Re-registering Microsoft.ContainerService resource provider..."
        az provider register --namespace Microsoft.ContainerService
        print_warning "Feature flag registration may take some time to propagate."
    else
        print_success "Feature flag is already registered."
    fi
    
    # Step 2: Create resource group
    print_info "Creating resource group '$resource_group' in location '$location'..."
    az group create --location "$location" --resource-group "$resource_group"
    print_success "Resource group created successfully."
    
    # Step 3: Deploy main.bicep
    local template_file="./test/infra/azure/main.bicep"
    if [[ ! -f "$template_file" ]]; then
        print_error "Template file not found: $template_file"
        print_info "Please ensure you're running this from the repository root."
        exit 1
    fi
    
    print_info "Deploying Bicep template..."
    local deployment_params="grafanaEnabled=$grafana_enabled"
    
    if [[ "$grafana_enabled" == "true" && -n "$grafana_admin_object_id" ]]; then
        deployment_params="$deployment_params grafanaAdminObjectId=$grafana_admin_object_id"
    fi
    
    print_info "Deployment parameters: $deployment_params"
    
    if az deployment group create \
        --resource-group "$resource_group" \
        --template-file "$template_file" \
        --parameters "$deployment_params"; then
        print_success "Deployment completed successfully!"
        print_info "Resource group: $resource_group"
        print_info "Location: $location"
        
        # Get AKS cluster name
        local aks_cluster
        aks_cluster=$(az aks list --resource-group "$resource_group" --query '[0].name' -o tsv 2>/dev/null || echo "")
        if [[ -n "$aks_cluster" ]]; then
            print_info "AKS Cluster: $aks_cluster"
            print_info "To connect to the cluster, run:"
            print_info "  az aks get-credentials --resource-group $resource_group --name $aks_cluster"
        fi
        
        if [[ "$grafana_enabled" == "true" ]]; then
            local grafana_endpoint
            grafana_endpoint=$(az grafana list --resource-group "$resource_group" --query '[0].properties.endpoint' -o tsv 2>/dev/null || echo "")
            if [[ -n "$grafana_endpoint" ]]; then
                print_info "Grafana Dashboard: $grafana_endpoint"
            fi
        fi
    else
        print_error "Deployment failed!"
        exit 1
    fi
}

# Cleanup long-running test cluster
cleanup_lrt_cluster() {
    print_info "Cleaning up long-running test cluster..."
    
    local resource_group="${LRT_RG:-}"
    
    if [[ -z "$resource_group" ]]; then
        print_error "LRT_RG environment variable is required for cleanup."
        print_info "Please set LRT_RG to the resource group name you want to delete."
        exit 1
    fi
    
    print_warning "This will delete the entire resource group: $resource_group"
    print_warning "This action cannot be undone!"
    
    # Check if resource group exists
    if ! az group exists --name "$resource_group" --output tsv | grep -q "true"; then
        print_warning "Resource group '$resource_group' does not exist."
        return 0
    fi
    
    print_info "Deleting resource group '$resource_group'..."
    if az group delete --name "$resource_group" --yes --no-wait; then
        print_success "Resource group deletion initiated."
        print_info "Deletion is running in the background and may take several minutes to complete."
    else
        print_error "Failed to initiate resource group deletion."
        exit 1
    fi
}

# Main function
main() {
    if [[ $# -eq 0 ]]; then
        print_error "No command specified."
        print_info "Available commands: deploy-lrt-cluster, cleanup-lrt-cluster"
        exit 1
    fi
    
    local command="$1"
    check_azure_cli
    
    case "$command" in
        "deploy-lrt-cluster")
            deploy_lrt_cluster
            ;;
        "delete-lrt-cluster")
            cleanup_lrt_cluster
            ;;
        *)
            print_error "Unknown command: $command"
            print_info "Available commands: deploy-lrt-cluster, delete-lrt-cluster"
            exit 1
            ;;
    esac
}

# Run main function with all arguments
main "$@"


set aks name
add some type of validation to ensure that LRT_RG name is set