#!/bin/bash

# ------------------------------------------------------------
# Copyright 2026 The Radius Authors.
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

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
TEST_ROOT=""

cleanup() {
    if [[ -n "${TEST_ROOT}" && -d "${TEST_ROOT}" ]]; then
        rm -rf "${TEST_ROOT}"
    fi
}
trap cleanup EXIT

setup() {
    TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/release-helm-test-XXXXXX")"
    mkdir -p "${TEST_ROOT}/bin" "${TEST_ROOT}/registry" \
        "${TEST_ROOT}/source/radius"
    printf 'apiVersion: v2\nname: radius\nversion: 0.61.0\n' \
        > "${TEST_ROOT}/source/radius/Chart.yaml"
    tar -czf "${TEST_ROOT}/radius-0.61.0.tgz" \
        -C "${TEST_ROOT}/source" radius
    : > "${TEST_ROOT}/calls"
    cat > "${TEST_ROOT}/bin/helm" << 'EOF'
#!/bin/bash
set -euo pipefail

echo "$*" >>"${HELM_CALLS}"
case "$1" in
    pull)
        if [[ "${HELM_FALSE_ABSENCE_ONCE:-}" == "true" &&
            ! -e "${HELM_REGISTRY}/false-absence" ]]; then
            touch "${HELM_REGISTRY}/false-absence"
            echo "manifest not found" >&2
            exit 1
        fi
        if [[ ! -f "${HELM_REGISTRY}/radius-0.61.0.tgz" ]]; then
            echo "manifest not found" >&2
            exit 1
        fi
        while [[ $# -gt 0 ]]; do
            if [[ "$1" == "--destination" ]]; then
                mkdir -p "$2"
                cp "${HELM_REGISTRY}/radius-0.61.0.tgz" "$2/"
                exit 0
            fi
            shift
        done
        ;;
    push)
        if [[ "${HELM_FAIL_PUSH_ONCE:-}" == "true" &&
            ! -e "${HELM_REGISTRY}/failed" ]]; then
            cp "$2" "${HELM_REGISTRY}/radius-0.61.0.tgz"
            touch "${HELM_REGISTRY}/failed"
            echo "503 Service Unavailable" >&2
            exit 1
        fi
        cp "$2" "${HELM_REGISTRY}/radius-0.61.0.tgz"
        ;;
    *) exit 2 ;;
esac
EOF
    chmod +x "${TEST_ROOT}/bin/helm"
}

run_publisher() {
    PATH="${TEST_ROOT}/bin:${PATH}" \
        HELM_CALLS="${TEST_ROOT}/calls" \
        HELM_REGISTRY="${TEST_ROOT}/registry" \
        RELEASE_RETRY_NO_SLEEP=true \
        bash "${SCRIPT_DIR}/publish-helm-chart.sh" \
            --archive "${TEST_ROOT}/radius-0.61.0.tgz" \
            --repository oci://example.test/charts \
            --name radius \
            --version 0.61.0 > /dev/null
}

main() {
    setup
    run_publisher
    run_publisher
    [[ "$(grep -c '^push ' "${TEST_ROOT}/calls")" == "1" ]]

    mkdir -p "${TEST_ROOT}/repacked"
    tar -xzf "${TEST_ROOT}/radius-0.61.0.tgz" \
        -C "${TEST_ROOT}/repacked"
    tar -czf "${TEST_ROOT}/radius-0.61.0.tgz" \
        -C "${TEST_ROOT}/repacked" radius
    run_publisher
    [[ "$(grep -c '^push ' "${TEST_ROOT}/calls")" == "1" ]]

    mkdir -p "${TEST_ROOT}/different/radius"
    printf 'apiVersion: v2\nname: radius\nversion: 9.9.9\n' \
        > "${TEST_ROOT}/different/radius/Chart.yaml"
    tar -czf "${TEST_ROOT}/registry/radius-0.61.0.tgz" \
        -C "${TEST_ROOT}/different" radius
    if run_publisher 2> /dev/null; then
        echo "publisher accepted a conflicting immutable chart" >&2
        exit 1
    fi

    rm -f "${TEST_ROOT}/registry/false-absence"
    if PATH="${TEST_ROOT}/bin:${PATH}" \
        HELM_CALLS="${TEST_ROOT}/calls" \
        HELM_REGISTRY="${TEST_ROOT}/registry" \
        HELM_FALSE_ABSENCE_ONCE=true \
        RELEASE_RETRY_NO_SLEEP=true \
        bash "${SCRIPT_DIR}/publish-helm-chart.sh" \
            --archive "${TEST_ROOT}/radius-0.61.0.tgz" \
            --repository oci://example.test/charts \
            --name radius \
            --version 0.61.0 > /dev/null 2>&1; then
        echo "publisher accepted a false absence before conflicting chart" >&2
        exit 1
    fi

    setup
    PATH="${TEST_ROOT}/bin:${PATH}" \
        HELM_CALLS="${TEST_ROOT}/calls" \
        HELM_REGISTRY="${TEST_ROOT}/registry" \
        HELM_FAIL_PUSH_ONCE=true \
        RELEASE_RETRY_NO_SLEEP=true \
        bash "${SCRIPT_DIR}/publish-helm-chart.sh" \
            --archive "${TEST_ROOT}/radius-0.61.0.tgz" \
            --repository oci://example.test/charts \
            --name radius \
            --version 0.61.0 > /dev/null
    [[ "$(grep -c '^push ' "${TEST_ROOT}/calls")" == "1" ]]
    echo "Helm chart publication tests passed"
}

main "$@"
