#!/bin/bash

# ============================================================================
# E2E runner for the "Private Registries & Repositories" demo (Linux / macOS).
#
# Automates the manual walkthrough in ../README.md: it creates the Radius group
# and namespaces, optionally publishes the sample Bicep recipe, deploys the
# environment + app for the selected scenario, and verifies the result.
#
# Configuration is supplied through environment variables (see usage). This
# keeps secrets out of the command line and mirrors the variables used in the
# README.
# ============================================================================

set -euo pipefail

SCRIPT_NAME="$(basename "$0")"
readonly SCRIPT_NAME
# Demo root is the parent of the directory holding this script.
DEMO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly DEMO_DIR
readonly BICEP_DIR="${DEMO_DIR}/bicep"
readonly RECIPES_DIR="${DEMO_DIR}/recipes"

readonly RAD_GROUP="demo-private-registries"
readonly BICEP_NAMESPACE="private-bicep-demo"
readonly TF_NAMESPACE="private-tf-demo"
readonly COMBINED_NAMESPACE="private-combined-demo"

# Default values
SCENARIO="all"
DO_CLEANUP="false"
PUBLISH_RECIPE="true"

usage() {
    cat <<EOF
Usage: ${SCRIPT_NAME} [OPTIONS]

Runs the private-registries E2E demo against a real cluster and real private
registries. Configure it with environment variables, then pick a scenario.

Options:
  -s, --scenario <name>   Scenario to run: bicep | terraform | combined | all
                          (default: all)
      --skip-publish      Skip 'rad bicep publish' (recipe already pushed)
  -c, --cleanup           Delete all demo resources and exit
  -h, --help              Show this help

Environment variables:
  Scenario 1 (bicep / combined):
    BICEP_REGISTRY            Private OCI registry host, e.g. myreg.azurecr.io
    BICEP_RECIPE              Full OCI path to the recipe, e.g.
                              myreg.azurecr.io/recipes/redis:latest
    BICEP_REGISTRY_USERNAME   BasicAuth username
    BICEP_REGISTRY_PASSWORD   BasicAuth password

  Scenario 2 (terraform / combined):
    TF_REGISTRY_HOST          Private Terraform registry host
    TF_RECIPE_LOCATION        Terraform module source (recipeLocation)
    TF_REGISTRY_TOKEN         Terraform registry token

Examples:
  ${SCRIPT_NAME} --scenario bicep
  ${SCRIPT_NAME} --scenario terraform --skip-publish
  ${SCRIPT_NAME} --cleanup
EOF
    exit 0
}

require_tools() {
    local tool
    for tool in rad kubectl; do
        if ! command -v "${tool}" >/dev/null 2>&1; then
            echo "Error: required tool '${tool}' not found on PATH" >&2
            exit 1
        fi
    done
}

require_vars() {
    local name
    local missing=()
    for name in "$@"; do
        if [[ -z "${!name:-}" ]]; then
            missing+=("${name}")
        fi
    done
    if [[ ${#missing[@]} -gt 0 ]]; then
        echo "Error: missing required variables: ${missing[*]}" >&2
        exit 1
    fi
}

ensure_namespace() {
    local ns="$1"
    if ! kubectl get namespace "${ns}" >/dev/null 2>&1; then
        echo "Creating namespace ${ns}"
        kubectl create namespace "${ns}"
    fi
}

setup_group() {
    # Group may already exist; suppress only the duplicate notice on stdout
    # while letting real errors surface. 'rad group switch' below validates it.
    rad group create "${RAD_GROUP}" >/dev/null || true
    rad group switch "${RAD_GROUP}"
}

run_bicep() {
    require_vars BICEP_REGISTRY BICEP_RECIPE \
        BICEP_REGISTRY_USERNAME BICEP_REGISTRY_PASSWORD
    ensure_namespace "${BICEP_NAMESPACE}"

    if [[ "${PUBLISH_RECIPE}" == "true" ]]; then
        echo "Publishing Bicep recipe to ${BICEP_RECIPE}"
        rad bicep publish \
            --file "${RECIPES_DIR}/redis-recipe.bicep" \
            --target "br:${BICEP_RECIPE}"
    fi

    echo "Deploying Scenario 1 (private Bicep registry)"
    rad deploy "${BICEP_DIR}/bicep-private-registry.bicep" \
        --parameters registryHostname="${BICEP_REGISTRY}" \
        --parameters recipeLocation="${BICEP_RECIPE}" \
        --parameters registryUsername="${BICEP_REGISTRY_USERNAME}" \
        --parameters registryPassword="${BICEP_REGISTRY_PASSWORD}"

    echo "Verifying Scenario 1"
    rad resource list Applications.Core/extenders
    kubectl get pods -n "${BICEP_NAMESPACE}"
}

run_terraform() {
    require_vars TF_REGISTRY_HOST TF_RECIPE_LOCATION TF_REGISTRY_TOKEN
    ensure_namespace "${TF_NAMESPACE}"

    echo "Deploying Scenario 2 (private Terraform registry)"
    rad deploy "${BICEP_DIR}/terraform-private-registry.bicep" \
        --parameters terraformRegistryHostname="${TF_REGISTRY_HOST}" \
        --parameters recipeLocation="${TF_RECIPE_LOCATION}" \
        --parameters terraformRegistryToken="${TF_REGISTRY_TOKEN}"

    echo "Verifying Scenario 2"
    rad resource list Applications.Core/extenders
    kubectl get pods -n "${TF_NAMESPACE}"
}

run_combined() {
    require_vars BICEP_REGISTRY BICEP_RECIPE \
        BICEP_REGISTRY_USERNAME BICEP_REGISTRY_PASSWORD \
        TF_REGISTRY_HOST TF_RECIPE_LOCATION TF_REGISTRY_TOKEN
    ensure_namespace "${COMBINED_NAMESPACE}"

    if [[ "${PUBLISH_RECIPE}" == "true" ]]; then
        echo "Publishing Bicep recipe to ${BICEP_RECIPE}"
        rad bicep publish \
            --file "${RECIPES_DIR}/redis-recipe.bicep" \
            --target "br:${BICEP_RECIPE}"
    fi

    echo "Deploying Scenario 3 (combined)"
    rad deploy "${BICEP_DIR}/combined.bicep" \
        --parameters terraformRegistryHostname="${TF_REGISTRY_HOST}" \
        --parameters terraformRecipeLocation="${TF_RECIPE_LOCATION}" \
        --parameters terraformRegistryToken="${TF_REGISTRY_TOKEN}" \
        --parameters bicepRegistryHostname="${BICEP_REGISTRY}" \
        --parameters bicepRegistryUsername="${BICEP_REGISTRY_USERNAME}" \
        --parameters bicepRegistryPassword="${BICEP_REGISTRY_PASSWORD}"

    echo "Verifying Scenario 3"
    rad resource show Radius.Core/environments combined-env
    rad resource list Radius.Core/terraformConfigs
    rad resource list Radius.Core/bicepConfigs
}

cleanup_demo() {
    echo "Cleaning up demo resources"
    rad group switch "${RAD_GROUP}" 2>/dev/null || true
    rad app delete "${BICEP_NAMESPACE}" --yes 2>/dev/null || true
    rad app delete "${TF_NAMESPACE}" --yes 2>/dev/null || true
    rad app delete "${COMBINED_NAMESPACE}" --yes 2>/dev/null || true
    rad group switch default 2>/dev/null || true
    rad group delete "${RAD_GROUP}" --yes 2>/dev/null || true
    kubectl delete namespace \
        "${BICEP_NAMESPACE}" "${TF_NAMESPACE}" "${COMBINED_NAMESPACE}" \
        --ignore-not-found
    echo "Cleanup complete"
}

main() {
    require_tools

    if [[ "${DO_CLEANUP}" == "true" ]]; then
        cleanup_demo
        exit 0
    fi

    echo "============================================================"
    echo "Private registries E2E demo - scenario: ${SCENARIO}"
    echo "============================================================"

    setup_group

    case "${SCENARIO}" in
        bicep) run_bicep ;;
        terraform) run_terraform ;;
        combined) run_combined ;;
        all)
            run_bicep
            run_terraform
            run_combined
            ;;
        *)
            echo "Error: unknown scenario '${SCENARIO}'" >&2
            exit 1
            ;;
    esac

    echo "============================================================"
    echo "Done. Run '${SCRIPT_NAME} --cleanup' to remove demo resources."
    echo "============================================================"
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -s | --scenario)
            SCENARIO="$2"
            shift 2
            ;;
        --skip-publish)
            PUBLISH_RECIPE="false"
            shift
            ;;
        -c | --cleanup)
            DO_CLEANUP="true"
            shift
            ;;
        -h | --help)
            usage
            ;;
        *)
            echo "Unknown option: $1" >&2
            exit 1
            ;;
    esac
done

main
