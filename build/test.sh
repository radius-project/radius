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
        exit 1
    fi
}

# Deploy long-running test cluster
deploy_lrt_cluster() {
    print_info "Deploying long-running test cluster to Azure..."
    
    # Default values (can be overridden by environment variables)
    local location="${LRT_AZURE_LOCATION:-westus3}"
    local resource_group="${LRT_RG:-$(whoami)-radius-lrt}"
    
    print_info "Configuration:"
    print_info "  Subscription: $(az account show --query "[name,id]" --output tsv | paste -sd,)"
    print_info "  Resource Group: $resource_group"
    print_info "  Location: $location"
    
    local feature_state
    feature_state=$(az feature show --namespace "Microsoft.ContainerService" --name "EnableImageCleanerPreview" --query properties.state -o tsv 2>/dev/null || echo "NotRegistered")
    if [[ "$feature_state" != "Registered" ]]; then
        print_warning "Feature flag is not registered. Registering now..."
        az feature register --namespace "Microsoft.ContainerService" --name "EnableImageCleanerPreview"
        az provider register --namespace Microsoft.ContainerService
    fi
    
    print_info "Creating resource group '$resource_group' in location '$location'..."
    az group create --location "$location" --resource-group "$resource_group" --output none
    print_success "Resource group created successfully."
    
    local template_file="./test/infra/azure/main.bicep"
    print_info "Deploying Bicep template..."
    if az deployment group create \
        --resource-group "$resource_group" \
        --template-file "$template_file"; then
        print_success "Deployment completed successfully!"
        
        # Get AKS cluster name
        local aks_cluster
        aks_cluster=$(az aks list --resource-group "$resource_group" --query '[0].name' -o tsv 2>/dev/null || echo "")
        if [[ -n "$aks_cluster" ]]; then
            print_info "AKS Cluster: $aks_cluster"
            print_info "To connect to the cluster, run:"
            print_info "  az aks get-credentials --resource-group $resource_group --name $aks_cluster"
            print_info "To delete the cluster, run:"
            print_info "  az group delete --name $resource_group --yes --no-wait"
        fi
    else
        print_error "Deployment failed!"
        exit 1
    fi
}

check_azure_cli
deploy_lrt_cluster
