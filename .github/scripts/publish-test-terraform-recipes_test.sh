#!/bin/bash

# Copyright 2026 The Radius Authors.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
# http://www.apache.org/licenses/LICENSE-2.0

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
work_dir="$(mktemp -d "${TMPDIR:-/tmp}/radius-terraform-publisher-test.XXXXXX")"
trap 'rm -rf -- "${work_dir}"' EXIT
export REAL_ZIP
REAL_ZIP="$(command -v zip)"
command -v unzip >/dev/null
mkdir -p "${work_dir}/bin" "${work_dir}/tmp" \
    "${work_dir}/recipes with spaces/first/nested" \
    "${work_dir}/recipes with spaces/.hidden" \
    "${work_dir}/recipes with spaces/empty" "${work_dir}/no-recipes"
printf 'resource "test" "example" {}\n' \
    >"${work_dir}/recipes with spaces/first/main.tf"
printf 'nested content\n' \
    >"${work_dir}/recipes with spaces/first/nested/file with spaces.txt"
printf 'hidden content\n' >"${work_dir}/recipes with spaces/first/.hidden-file"
printf 'hidden recipe\n' >"${work_dir}/recipes with spaces/.hidden/main.tf"
touch "${work_dir}/recipes with spaces/ignored-file"

cat >"${work_dir}/bin/kubectl" <<'EOF'
#!/bin/bash
set -euo pipefail
printf '%s\n' "$@" >"${MOCK_STATE}/$1"
[[ "$2" == configmap && "$3" == test-content &&
    "$4" == --namespace && "$5" == test-ns ]]
case "$1" in
delete)
    [[ $# == 6 && "$6" == --ignore-not-found=true ]]
    if [[ "${MOCK_FAILURE}" == delete ]]; then
        echo "intentional delete failure" >&2
        exit 17
    fi
    ;;
create)
    [[ -f "${MOCK_STATE}/delete" ]]
    if [[ "${MOCK_FAILURE}" == create ]]; then
        echo "intentional create failure" >&2
        exit 18
    fi
    shift 5
    for entry in "$@"; do
        [[ "${entry}" == --from-file=*.zip=* ]]
        entry="${entry#--from-file=}"
        cp "${entry#*=}" "${MOCK_STATE}/${entry%%=*}"
    done
    ;;
*) exit 99 ;;
esac
EOF
cat >"${work_dir}/bin/zip" <<'EOF'
#!/bin/bash
set -euo pipefail
if [[ "${MOCK_FAILURE}" == zip ]]; then
    echo "intentional zip failure" >&2
    exit 23
fi
exec "${REAL_ZIP}" "$@"
EOF
chmod +x "${work_dir}/bin/kubectl" "${work_dir}/bin/zip"
export PATH="${work_dir}/bin:${PATH}"
export TMPDIR="${work_dir}/tmp"
export MOCK_FAILURE=""
shopt -s dotglob nullglob

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

run_case() {
    local expected="$1" status=0
    shift
    export MOCK_STATE
    MOCK_STATE="$(mktemp -d "${work_dir}/case.XXXXXX")"
    bash "${SCRIPT_DIR}/publish-test-terraform-recipes.sh" "$@" \
        >"${MOCK_STATE}/output" 2>&1 || status=$?
    if [[ "${status}" != "${expected}" ]]; then
        cat "${MOCK_STATE}/output"
        fail "expected exit ${expected}, got ${status}"
    fi
    local remaining=("${TMPDIR}"/*)
    ((${#remaining[@]} == 0)) || fail "temporary archives were not cleaned up"
}

run_case 1
grep -q "Usage:" "${MOCK_STATE}/output"
run_case 1 "${work_dir}/missing" test-ns test-content
[[ ! -e "${MOCK_STATE}/delete" ]]
run_case 1 "${work_dir}/no-recipes" test-ns test-content
grep -q "No recipes found" "${MOCK_STATE}/output"
[[ ! -e "${MOCK_STATE}/delete" ]]

export GITHUB_STEP_SUMMARY="${work_dir}/summary"
printf 'existing summary\n' >"${GITHUB_STEP_SUMMARY}"
run_case 0 "${work_dir}/recipes with spaces" test-ns test-content
archives=("${MOCK_STATE}"/*.zip)
((${#archives[@]} == 3)) || fail "expected one archive per recipe directory"
[[ "$(unzip -p "${MOCK_STATE}/first.zip" main.tf)" == \
    'resource "test" "example" {}' ]]
[[ "$(unzip -p "${MOCK_STATE}/first.zip" 'nested/file with spaces.txt')" == \
    'nested content' ]]
[[ "$(unzip -p "${MOCK_STATE}/first.zip" .hidden-file)" == 'hidden content' ]]
[[ "$(unzip -p "${MOCK_STATE}/.hidden.zip" main.tf)" == 'hidden recipe' ]]
status=0
empty_listing="$(unzip -Z1 "${MOCK_STATE}/empty.zip" 2>&1)" || status=$?
[[ "${status}" == 1 && "${empty_listing}" == "Empty zipfile." ]]
grep -Fxq "existing summary" "${GITHUB_STEP_SUMMARY}"
grep -Fxq "Created configmap: test-ns/test-content" "${GITHUB_STEP_SUMMARY}"
grep -Fxq "Created configmap: test-ns/test-content" "${MOCK_STATE}/output"
unset GITHUB_STEP_SUMMARY
(
    cd "${work_dir}"
    run_case 0 "recipes with spaces" test-ns test-content
)

for failure in zip delete create; do
    export MOCK_FAILURE="${failure}"
    case "${failure}" in
    zip) expected=23 ;;
    delete) expected=17 ;;
    create) expected=18 ;;
    esac
    run_case "${expected}" "${work_dir}/recipes with spaces" test-ns test-content
    grep -q "intentional ${failure} failure" "${MOCK_STATE}/output"
    if [[ "${failure}" == zip ]]; then
        [[ ! -e "${MOCK_STATE}/delete" ]]
    fi
    if [[ "${failure}" != create ]]; then
        [[ ! -e "${MOCK_STATE}/create" ]]
    fi
    if grep -q "Created configmap:" "${MOCK_STATE}/output"; then
        fail "reported success after ${failure} failed"
    fi
done

status=0
PATH="${work_dir}/no-tools" "${BASH}" \
    "${SCRIPT_DIR}/publish-test-terraform-recipes.sh" \
    "${work_dir}/recipes with spaces" test-ns test-content \
    >"${work_dir}/missing-tool-output" 2>&1 || status=$?
[[ "${status}" == 1 ]]
grep -q "Missing required tool: zip" "${work_dir}/missing-tool-output"

echo "Terraform recipe publisher tests passed"
