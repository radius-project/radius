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

    # Check if jq is installed
    if ! command -v jq &> /dev/null; then
        print_error "jq is required but not installed."
        print_info "Please install jq to parse JSON responses."
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
            # The gh CLI command show below is simpler but much slower than using curl
            # gh run delete --repo "$REPO" "$id"
            if ! curl -sL -X DELETE \
                -H "Accept: application/vnd.github+json" \
                -H "Authorization: Bearer $GITHUB_TOKEN" \
                -H "X-GitHub-Api-Version: 2022-11-28" \
                "https://api.github.com/repos/$REPO/actions/runs/$id"; then
                print_error "Failed to delete workflow run with ID: $id"
            fi
        done

        echo "Batch completed. Checking for more workflow runs..."
    done

    print_success "Deleted all workflow runs."
}

# Toggle workflows (enable or disable)
toggle_workflows() {
    
    if [[ $# -ne 2 ]]; then
        print_error "Usage: toggle_workflows <action> <repo>"
        exit 1
    fi

    local action="$1"
    local repo="$2"

    if [[ "$action" != "enable" && "$action" != "disable" ]]; then
        print_error "Invalid action: $action. Must be 'enable' or 'disable'"
        exit 1
    fi

    check_org_restriction "$action all workflows"
    
    local action_verb action_past_tense gh_command gh_state
    if [[ "$action" == "enable" ]]; then
        action_verb="Enabling"
        action_past_tense="enabled"
        gh_command="enable"
        gh_state="disabled_manually"
    else
        action_verb="Disabling"
        action_past_tense="disabled"
        gh_command="disable"
        gh_state="active"
    fi

    print_info "$action_verb all workflows in repository '$repo' with state '$gh_state'."

    # Get workflows with their current state
    local workflows
    workflows=$(gh workflow list --repo "$repo" --all --json name,state | jq -r '.[] | select(.state == "'"$gh_state"'") | .name')

    if [[ -z "$workflows" ]]; then
        print_warning "No workflows found that need to be $action_past_tense."
        return 0
    fi
    
    # Enable/disable each workflow that needs the action
    while read -r name; do
        if [[ -n "$name" ]]; then
            print_info "$action_verb workflow: $name"
            gh workflow "$gh_command" --repo "$repo" "$name"
        fi
    done <<< "$workflows"
    
    print_success "All workflows have been $action_past_tense."
}

# Main function
main() {
    
    local available_commands="Available commands: enable-all, disable-all, delete-all-runs"

    # Check if command is provided
    if [[ $# -eq 0 ]]; then
        print_error "No command provided."
        print_info "Usage: $0 <command>"    
        print_info "$available_commands"
        exit 1
    fi

    local command="$1"
    check_gh_cli
    local repo
    repo=$(get_repo_info)

    case "$command" in
        "enable-all")
            toggle_workflows "enable" "$repo"
            ;;
        "disable-all")
            toggle_workflows "disable" "$repo"
            ;;
        "delete-all-runs")
            delete_all_runs "$repo"
            ;;
        *)
            print_error "Unknown command: $command"
            print_info "$available_commands"
            exit 1
            ;;
    esac
}

# Run main function with all arguments
main "$@"
