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
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

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

# Check if we're in a radius-project organization repo for security
check_org_restriction() {
    local operation="$1"
    local repo_url
    repo_url=$(git remote get-url origin 2>/dev/null || echo "")
    
    if [[ "$repo_url" == *"radius-project/"* ]]; then
        print_info "Operation '$operation' should only be used on personal forks."
        exit 1
    fi
}

# Check if GitHub CLI is installed and authenticated
check_gh_cli() {
    if ! command -v gh &> /dev/null; then
        print_error "GitHub CLI (gh) is required but not installed."
        print_info "Please install it from: https://cli.github.com/"
        exit 1
    fi

    # Check if GITHUB_TOKEN or GH_TOKEN environment variables are already set
    if [[ -n "${GITHUB_TOKEN:-}" ]] || [[ -n "${GH_TOKEN:-}" ]]; then
        return 0
    fi

    # Check if user is authenticated and get token
    local token
    if ! token=$(gh auth token 2>/dev/null); then
        print_error "GitHub CLI is not authenticated."
        print_info "Please run 'gh auth login' to authenticate."
        exit 1
    fi
    
    if [[ -z "$token" ]]; then
        print_error "Unable to retrieve GitHub authentication token."
        print_info "Please run 'gh auth login' to authenticate."
        exit 1
    fi
    
    export GITHUB_TOKEN="$token"
}

# Get repository info
get_repo_info() {
    
    # Verify we're in a git repository
    if ! git rev-parse --git-dir &> /dev/null; then
        print_error "Not in a git repository."
        exit 1
    fi

    local repo_url
    repo_url=$(git remote get-url origin 2>/dev/null)
    
    if [[ -z "$repo_url" ]]; then
        print_error "Not in a git repository or no origin remote found."
        exit 1
    fi
    
    # Check if it's a GitHub repository
    if [[ "$repo_url" != *"github.com"* ]]; then
        print_error "Repository is not hosted on GitHub."
        exit 1
    fi

    # Remove https://github.com/ from the beginning
    repo_url="${repo_url#https://github.com/}"
    
    # Remove .git from the end if present
    repo_url="${repo_url%.git}"
    
    echo "$repo_url"
}

# Dispatch a workflow
dispatch_workflow() {
    local workflow_name="$1"
    local branch="$2"
    
    print_info "Dispatching workflow '$workflow_name' on branch '$branch'..."
    
    # Check if workflow file exists
    if [[ ! -f ".github/workflows/$workflow_name" ]]; then
        print_error "Workflow file '.github/workflows/$workflow_name' not found."
        print_info "Available workflows:"
        list_workflows
        exit 1
    fi
    
    # Attempt to dispatch the workflow
    if gh workflow run "$workflow_name" --ref "$branch"; then
        print_success "Workflow '$workflow_name' dispatched successfully on branch '$branch'."
        print_info "You can view the workflow run at: $(gh repo view --web --branch "$branch" 2>/dev/null || echo "GitHub repository page")"
    else
        print_error "Failed to dispatch workflow '$workflow_name'."
        print_info "Make sure the workflow supports manual triggering (workflow_dispatch event)."
        exit 1
    fi
}

# List all workflows
list_workflows() {
    local REPO_NAME=$1
    print_info "Available workflows for $REPO_NAME:"

    local workflows
    workflows=$(gh workflow list --all --repo "$REPO_NAME" --json name,path,state --jq '.[] | "\(.path)|\(.name)|\(.state)"' 2>/dev/null)

    if [[ -n "$workflows" ]]; then
        while IFS='|' read -r path name state; do
            local filename
            filename=$(basename "$path")
            printf "  %-30s %-40s %s\n" "$filename" "($name)" "[$state]"
        done <<< "$workflows"
    else
        print_warning "No workflows found or unable to retrieve workflow information."
    fi
}

# Disable a workflow
disable_workflow() {
    local workflow_name="$1"
    local REPO_NAME="$2"

    check_org_restriction "disable workflow"
    
    print_info "Disabling workflow '$workflow_name'..."

    if gh workflow disable --repo "$REPO_NAME" "$workflow_name"; then
        print_success "Workflow '$workflow_name' disabled successfully."
    else
        print_error "Failed to disable workflow '$workflow_name'."
        print_info "Make sure the workflow name is correct. Use 'make workflow-list' to see available workflows."
        exit 1
    fi
}

# Enable a workflow
enable_workflow() {
    local workflow_name="$1"
    local REPO_NAME="$2"
    
    print_info "Enabling workflow '$workflow_name'..."

    if gh workflow enable --repo "$REPO_NAME" "$workflow_name"; then
        print_success "Workflow '$workflow_name' enabled successfully."
    else
        print_error "Failed to enable workflow '$workflow_name'."
        print_info "Make sure the workflow name is correct. Use 'make workflow-list' to see available workflows."
        exit 1
    fi
}

# Disable all triggered workflows
disable_triggered_workflows() {
    check_org_restriction "disable triggered workflows"
    
    print_info "Disabling all workflows triggered by events..."
    
    local disabled_count=0
    
    # Get list of workflows and their trigger events
    for workflow_file in .github/workflows/*.{yml,yaml}; do
        if [[ -f "$workflow_file" ]]; then
            local filename=$(basename "$workflow_file")
            
            # Check if workflow has triggers other than workflow_dispatch
            if grep -qE "^\s*(on:|push:|pull_request:|schedule:|workflow_run:)" "$workflow_file" && \
               ! grep -qE "^\s*workflow_dispatch:\s*$" "$workflow_file" || \
               grep -qE "^\s*schedule:" "$workflow_file"; then
                
                print_info "Disabling '$filename'..."
                if gh workflow disable "$filename" 2>/dev/null; then
                    print_success "  Disabled '$filename'"
                    ((disabled_count++))
                else
                    print_warning "  Failed to disable '$filename' (may already be disabled)"
                fi
            fi
        fi
    done
    
    if [[ $disabled_count -gt 0 ]]; then
        print_success "Disabled $disabled_count triggered workflows."
    else
        print_info "No triggered workflows found to disable."
    fi
}

# Delete all workflow runs
delete_all_runs() {
    check_org_restriction "delete all workflow runs"
    
    print_info "Deleting all workflow runs..."
    
    # Check if repository parameter is provided
    if [ $# -eq 0 ]; then
        echo "Usage: $0 <repository>"
        exit 1
    fi

    local REPO="$1"
    
    while true; do
        # Get a batch of workflow run IDs
        run_ids=$(gh run list --repo "$REPO" -L 30 --json databaseId --jq '.[].databaseId')
        
        # Check if we got any results
        if [[ -z "$run_ids" ]]; then
            echo "No more workflow runs found. Exiting."
            break
        fi
        
        # Process each ID in the current batch
        echo "$run_ids" | while read id; do
            echo "Deleting workflow run with ID: $id"
            # The gh CLI command is simpler but much slower than using curl
            #gh run delete --repo "$REPO" "$id"
            curl -sL -X DELETE \
                -H "Accept: application/vnd.github+json" \
                -H "Authorization: Bearer $GITHUB_TOKEN" \
                -H "X-GitHub-Api-Version: 2022-11-28" \
                "https://api.github.com/repos/$REPO/actions/runs/$id"
        done
        
        echo "Batch completed. Checking for more workflow runs..."
    done

    print_success "Deleted all workflow runs."
}

# Main function
main() {
    local command="$1"
    check_gh_cli
    local repo
    repo=$(get_repo_info)

    case "$command" in
        "dispatch")
            if [[ $# -lt 3 ]]; then
                print_error "Usage: $0 dispatch <workflow-name> <branch>"
                exit 1
            fi
            dispatch_workflow "$2" "$3"
            ;;
        "list")
            list_workflows "$repo"
            ;;
        "disable")
            if [[ $# -lt 2 ]]; then
                print_error "Usage: $0 disable <workflow-name>"
                exit 1
            fi
            disable_workflow "$2" "$repo"
            ;;
        "enable")
            if [[ $# -lt 2 ]]; then
                print_error "Usage: $0 enable <workflow-name>"
                exit 1
            fi
            enable_workflow "$2" "$repo"
            ;;
        "disable-triggered")
            disable_triggered_workflows
            ;;
        "delete-all-runs")
            delete_all_runs "$repo"
            ;;
        *)
            print_error "Unknown command: $command"
            print_info "Available commands: dispatch, list, disable, enable, disable-triggered, delete-all-runs"
            exit 1
            ;;
    esac
}

# Run main function with all arguments
main "$@"
