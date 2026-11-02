#!/bin/bash

# Copyright 2026 The Radius Authors.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
# http://www.apache.org/licenses/LICENSE-2.0

# Publish Terraform recipe ZIPs as a ConfigMap for the test module server.
set -euo pipefail

if [[ $# -ne 3 ]]; then
    echo "Usage: $0 <recipe root> <namespace> <config map name>" >&2
    exit 1
fi

for tool in zip kubectl; do
    command -v "${tool}" >/dev/null || {
        echo "Missing required tool: ${tool}" >&2
        exit 1
    }
done

recipe_root="$(cd -- "$1" && pwd)"
readonly recipe_root
readonly namespace="$2"
readonly config_map_name="$3"

log() {
    printf '%s\n' "$1"
    printf '%s\n' "$1" >>"${GITHUB_STEP_SUMMARY:-/dev/null}"
}

log "Publishing recipes from ${recipe_root} to ${namespace}/${config_map_name}"
shopt -s dotglob nullglob
recipe_dirs=("${recipe_root}"/*/)
if ((${#recipe_dirs[@]} == 0)); then
    echo "No recipes found in ${recipe_root}" >&2
    exit 1
fi

temp_dir="$(mktemp -d "${TMPDIR:-/tmp}/radius-test-terraform-recipes.XXXXXX")"
trap 'rm -rf -- "${temp_dir}"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
temp_dir="$(cd -- "${temp_dir}" && pwd)"

config_entries=()
for recipe_dir in "${recipe_dirs[@]}"; do
    recipe_dir="${recipe_dir%/}"
    recipe_name="${recipe_dir##*/}"
    output_filename="${temp_dir}/${recipe_name}.zip"
    log "Processing recipe: ${recipe_dir}"
    (
        cd -- "${recipe_dir}"
        # Explicitly include '.' so empty recipe directories yield empty ZIPs.
        zip -qr "${output_filename}" . -i '*' .
    )
    log "Created zip file: ${output_filename}"
    config_entries+=("--from-file=${recipe_name}.zip=${output_filename}")
done

kubectl delete configmap "${config_map_name}" \
    --namespace "${namespace}" --ignore-not-found=true
kubectl create configmap "${config_map_name}" \
    --namespace "${namespace}" "${config_entries[@]}"
log "Created configmap: ${namespace}/${config_map_name}"
