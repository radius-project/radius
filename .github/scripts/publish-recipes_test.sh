#!/bin/bash

# Copyright 2026 The Radius Authors.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
# http://www.apache.org/licenses/LICENSE-2.0

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
work_dir="$(mktemp -d "${TMPDIR:-/tmp}/radius-publish-recipes-test.XXXXXX")"
trap 'rm -rf "${work_dir}"' EXIT
mkdir -p "${work_dir}/bin" "${work_dir}/recipes with spaces/nested"
for index in {1..8}; do
    touch "${work_dir}/recipes with spaces/recipe-${index}.bicep"
done
touch "${work_dir}/recipes with spaces/nested/recipe-9.bicep" \
    "${work_dir}/recipes with spaces/_module.bicep"
cat >"${work_dir}/bin/rad" <<'EOF'
#!/bin/bash
set -euo pipefail
[[ "$#" == 6 && "$1 $2 $3" == "bicep publish --file" && "$5" == --target ]]
[[ "$4" == "${RECIPE_DIRECTORY}/"* ]]
name="${4##*/}"
[[ "$6" == "br:example.local/recipes/${name%.bicep}:test" ]]
mkdir "${MOCK_STATE}/active/${name}"
trap 'printf "%s\n" "${name}" >>"${MOCK_STATE}/finished"; rmdir "${MOCK_STATE}/active/${name}"' EXIT
printf '%s\n' "${name}" >>"${MOCK_STATE}/started"
active=("${MOCK_STATE}"/active/*)
((${#active[@]} <= MOCK_PARALLELISM))
if ((${#active[@]} > 1)); then
    touch "${MOCK_STATE}/overlap"
fi
sleep 0.1
if [[ "${name}" == "${MOCK_FAILURE}" ]]; then
    echo "intentional publish failure: ${name}" >&2
    exit 77
fi
if [[ "${name}" == "${MOCK_SIGNAL_RECIPE}" ]]; then
    kill -TERM "${PPID}"
fi
EOF
chmod +x "${work_dir}/bin/rad"
export PATH="${work_dir}/bin:${PATH}"
export RECIPE_DIRECTORY="${work_dir}/recipes with spaces"
export MOCK_FAILURE="" MOCK_SIGNAL_RECIPE=""
run_case() {
    local expected="$1" status=0
    export MOCK_STATE
    MOCK_STATE="$(mktemp -d "${work_dir}/state.XXXXXX")"
    mkdir "${MOCK_STATE}/active"
    export GITHUB_STEP_SUMMARY="${MOCK_STATE}/summary"
    bash "${SCRIPT_DIR}/publish-recipes.sh" "${RECIPE_DIRECTORY}" \
        example.local/recipes test >"${MOCK_STATE}/output" 2>&1 || status=$?
    [[ "${status}" == "${expected}" ]] || {
        cat "${MOCK_STATE}/output"
        echo "Expected ${expected}, got ${status}" >&2
        exit 1
    }
    if [[ -f "${MOCK_STATE}/started" ]]; then
        [[ "$(wc -l <"${MOCK_STATE}/started")" == "$(wc -l <"${MOCK_STATE}/finished")" ]]
    fi
    remaining=("${MOCK_STATE}"/active/*)
    [[ ! -e "${remaining[0]}" ]]
}
for parallelism in default 2 3 4; do
    unset BICEP_RECIPE_PUBLISH_PARALLELISM
    export MOCK_PARALLELISM=1
    if [[ "${parallelism}" != default ]]; then
        export BICEP_RECIPE_PUBLISH_PARALLELISM="${parallelism}"
        export MOCK_PARALLELISM="${parallelism}"
    fi
    run_case 0
    [[ "$(wc -l <"${GITHUB_STEP_SUMMARY}")" -eq 10 ]]
    for index in {1..9}; do
        [[ "$(grep -cx "recipe-${index}.bicep" "${MOCK_STATE}/started")" == 1 ]]
    done
    if [[ "${parallelism}" == default ]]; then
        [[ ! -e "${MOCK_STATE}/overlap" ]]
    else
        [[ -e "${MOCK_STATE}/overlap" ]]
    fi
done
export BICEP_RECIPE_PUBLISH_PARALLELISM=4 MOCK_PARALLELISM=4
export MOCK_FAILURE=recipe-1.bicep
run_case 77
grep -q 'intentional publish failure' "${MOCK_STATE}/output"
export MOCK_FAILURE="" MOCK_SIGNAL_RECIPE=recipe-1.bicep
run_case 143
export MOCK_SIGNAL_RECIPE=""
# Interrupt the publisher after it starts a worker but before it records the
# worker's process ID. The publisher must still wait for that worker.
cat >"${work_dir}/signal-before-record.sh" <<'EOF'
trap 'if [[ "${BASH_COMMAND}" == "publish_pids+=("* && ! -e "${MOCK_STATE}/signaled" ]]; then touch "${MOCK_STATE}/signaled"; kill -TERM "$$"; fi' DEBUG
EOF
export BASH_ENV="${work_dir}/signal-before-record.sh"
run_case 143
unset BASH_ENV
[[ -e "${MOCK_STATE}/signaled" ]]
[[ "$(wc -l <"${MOCK_STATE}/finished")" -eq 1 ]]
for parallelism in 0 5 invalid; do
    export BICEP_RECIPE_PUBLISH_PARALLELISM="${parallelism}"
    run_case 2
    [[ ! -e "${MOCK_STATE}/started" ]]
done
touch "${RECIPE_DIRECTORY}/nested/recipe-1.bicep"
export BICEP_RECIPE_PUBLISH_PARALLELISM=4 MOCK_PARALLELISM=4
run_case 2
grep -q 'Duplicate publish reference' "${MOCK_STATE}/output"
unset BICEP_RECIPE_PUBLISH_PARALLELISM
export MOCK_PARALLELISM=1
run_case 0
[[ "$(grep -cx recipe-1.bicep "${MOCK_STATE}/started")" == 2 ]]
echo "recipe publisher tests passed"
