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
readonly SCRIPT="${SCRIPT_DIR}/release-oci-artifacts.sh"
TEST_ROOT=""
PASS=0
FAIL=0
readonly SOURCE_SHA="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

cleanup() {
    if [[ -n "${TEST_ROOT}" && -d "${TEST_ROOT}" ]]; then
        rm -rf "${TEST_ROOT}"
    fi
}
trap cleanup EXIT

fail_test() {
    echo "  ASSERT FAILED: $1"
    ((++FAIL))
}

setup_fixture() {
    if [[ -n "${TEST_ROOT}" && -d "${TEST_ROOT}" ]]; then
        rm -rf "${TEST_ROOT}"
    fi
    TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/release-oci-test-XXXXXX")"
    mkdir -p "${TEST_ROOT}/bin" "${TEST_ROOT}/dist/linux" \
        "${TEST_ROOT}/dist/windows"
    printf 'linux binary\n' > "${TEST_ROOT}/dist/linux/rad"
    printf 'windows binary\n' > "${TEST_ROOT}/dist/windows/rad.exe"
    cat > "${TEST_ROOT}/targets.json" << 'EOF'
{
  "cliAssets": [
    {"name":"rad_linux_amd64","os":"linux","arch":"amd64"},
    {"name":"rad_windows_amd64.exe","os":"windows","arch":"amd64"}
  ],
  "images": [
    {"name":"applications-rp","category":"production","radiusBuild":true},
    {"name":"ucpd","category":"production","radiusBuild":true}
  ]
}
EOF
    cat > "${TEST_ROOT}/artifacts.json" << EOF
[
  {
    "name":"rad_linux_amd64",
    "type":"Binary",
    "path":"${TEST_ROOT}/dist/linux/rad",
    "extra":{"ID":"rad"}
  },
  {
    "name":"rad_windows_amd64.exe",
    "type":"Binary",
    "path":"${TEST_ROOT}/dist/windows/rad.exe",
    "extra":{"ID":"rad"}
  }
]
EOF
    : > "${TEST_ROOT}/registry-state"
    : > "${TEST_ROOT}/calls"
    write_fake_oras
    write_fake_docker
}

digest_for() {
    printf '%s' "$1" | sha256sum | cut -d ' ' -f 1
}

write_fake_oras() {
    cat > "${TEST_ROOT}/bin/oras" << 'EOF'
#!/bin/bash
set -euo pipefail

state="${FAKE_REGISTRY_STATE}"
calls="${FAKE_REGISTRY_CALLS}"
command="$1"
shift
echo "oras ${command} $*" >>"${calls}"

case "${command}" in
    push)
        reference="$1"
        if [[ "${FAKE_FAIL_PUSH_ONCE:-}" == "true" &&
            ! -e "${state}.push-failed" ]]; then
            touch "${state}.push-failed"
            echo "503 Service Unavailable" >&2
            exit 1
        fi
        digest="sha256:$(printf '%s' "${reference}" | sha256sum | cut -d ' ' -f 1)"
        mkdir -p "${state}.blobs"
        blob="${state}.blobs/${digest#sha256:}"
        cp "$2" "${blob}"
        printf '%s\t%s\t%s\t%s\n' \
            "${reference}" "${digest}" "${blob}" "$(basename "$2")" \
            >>"${state}"
        ;;
    resolve)
        reference="$1"
        digest="$(awk -F '\t' -v ref="${reference}" '$1 == ref { value=$2 } END { print value }' "${state}")"
        if [[ "${FAKE_CORRUPT_ALIAS:-}" != "" && "${reference}" == *":${FAKE_CORRUPT_ALIAS}" ]]; then
            digest="sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
        fi
        if [[ "${reference}" == "${FAKE_CORRUPT_REFERENCE:-}" ]]; then
            digest="sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
        fi
        if [[ -z "${digest}" ]]; then
            echo "manifest not found" >&2
            exit 1
        fi
        printf '%s\n' "${digest}"
        ;;
    pull)
        reference="$1"
        shift
        if [[ "${FAKE_FAIL_PULL_ONCE:-}" == "true" &&
            ! -e "${state}.pull-failed" ]]; then
            touch "${state}.pull-failed"
            echo "503 Service Unavailable" >&2
            exit 1
        fi
        output_dir=""
        while [[ $# -gt 0 ]]; do
            if [[ "$1" == "--output" ]]; then
                output_dir="$2"
                break
            fi
            shift
        done
        record="$(awk -F '\t' -v ref="${reference}" '$1 == ref { value=$0 } END { print value }' "${state}")"
        if [[ -z "${record}" ]]; then
            echo "manifest not found" >&2
            exit 1
        fi
        blob="$(cut -f 3 <<<"${record}")"
        name="$(cut -f 4 <<<"${record}")"
        mkdir -p "${output_dir}"
        cp "${blob}" "${output_dir}/${name}"
        ;;
    tag)
        source="$1"
        shift
        if [[ "${FAKE_FAIL_TAG_ONCE:-}" == "true" &&
            ! -e "${state}.tag-failed" ]]; then
            touch "${state}.tag-failed"
            echo "429 Too Many Requests" >&2
            exit 1
        fi
        repository="${source%@*}"
        digest="${source##*@}"
        record="$(awk -F '\t' -v digest="${digest}" '$2 == digest && $3 != "" { value=$0 } END { print value }' "${state}")"
        blob="$(cut -f 3 <<<"${record}")"
        name="$(cut -f 4 <<<"${record}")"
        for alias in "$@"; do
            printf '%s\t%s\t%s\t%s\n' \
                "${repository}:${alias}" "${digest}" "${blob}" "${name}" \
                >>"${state}"
        done
        ;;
    *) exit 2 ;;
esac
EOF
    chmod +x "${TEST_ROOT}/bin/oras"
}

write_fake_docker() {
    cat > "${TEST_ROOT}/bin/docker" << 'EOF'
#!/bin/bash
set -euo pipefail

state="${FAKE_REGISTRY_STATE}"
calls="${FAKE_REGISTRY_CALLS}"
echo "docker $*" >>"${calls}"

if [[ "$1 $2 $3" == "buildx imagetools create" ]]; then
    shift 3
    if [[ "${FAKE_FAIL_IMAGE_ALIAS_ONCE:-}" == "true" &&
        ! -e "${state}.image-alias-failed" ]]; then
        touch "${state}.image-alias-failed"
        echo "503 Service Unavailable" >&2
        exit 1
    fi
    tags=()
    while [[ "$1" == "--tag" ]]; do
        tags+=("$2")
        shift 2
    done
    source="$1"
    digest="${source##*@}"
    for tag in "${tags[@]}"; do
        printf '%s\t%s\n' "${tag}" "${digest}" >>"${state}"
    done
    exit 0
fi

if [[ "$1 $2 $3" == "buildx imagetools inspect" ]]; then
    reference="${@: -1}"
    if [[ "${FAKE_IMAGE_LOOKUP_ERROR:-}" == "silent" ]]; then
        exit 1
    fi
    if [[ "${FAKE_IMAGE_LOOKUP_ERROR:-}" == "credentials" ]]; then
        echo "error getting credentials" >&2
        exit 1
    fi
    digest="$(awk -F '\t' -v ref="${reference}" '$1 == ref { value=$2 } END { print value }' "${state}")"
    if [[ "${reference}" == "${FAKE_CORRUPT_REFERENCE:-}" ]]; then
        digest="sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
    fi
    if [[ -z "${digest}" ]]; then
        echo "manifest not found" >&2
        exit 1
    fi
    printf '{"manifest":{"digest":"%s"}}\n' "${digest}"
    exit 0
fi

exit 2
EOF
    chmod +x "${TEST_ROOT}/bin/docker"
}

run_script() {
    PATH="${TEST_ROOT}/bin:${PATH}" \
        FAKE_REGISTRY_STATE="${TEST_ROOT}/registry-state" \
        FAKE_REGISTRY_CALLS="${TEST_ROOT}/calls" \
        RELEASE_RETRY_NO_SLEEP=true \
        RELEASE_SOURCE_SHA="${SOURCE_SHA}" \
        GORELEASER_PARITY_TARGETS="${TEST_ROOT}/targets.json" \
        bash "${SCRIPT}" "$@"
}

stage_cli() {
    run_script stage-cli \
        --registry example.test/radius \
        --version 0.61.0 \
        --artifacts "${TEST_ROOT}/artifacts.json" \
        --output "${TEST_ROOT}/cli-lock.json"
}

write_image_lock() {
    local app_digest
    local ucpd_digest

    app_digest="sha256:$(digest_for applications-rp)"
    ucpd_digest="sha256:$(digest_for ucpd)"
    cat > "${TEST_ROOT}/image-lock.json" << EOF
[
  {
    "name":"applications-rp",
    "reference":"example.test/radius/applications-rp:0.61.0",
    "digest":"${app_digest}",
    "sourceSha":"${SOURCE_SHA}",
    "immutableReference":"example.test/radius/applications-rp@${app_digest}"
  },
  {
    "name":"ucpd",
    "reference":"example.test/radius/ucpd:0.61.0",
    "digest":"${ucpd_digest}",
    "sourceSha":"${SOURCE_SHA}",
    "immutableReference":"example.test/radius/ucpd@${ucpd_digest}"
  }
]
EOF
    printf '%s\t%s\n' \
        "example.test/radius/applications-rp:0.61.0" "${app_digest}" \
        >> "${TEST_ROOT}/registry-state"
    printf '%s\t%s\n' \
        "example.test/radius/ucpd:0.61.0" "${ucpd_digest}" \
        >> "${TEST_ROOT}/registry-state"
}

test_stages_cli_artifacts() {
    setup_fixture
    stage_cli

    if [[ "$(jq -r '.version' "${TEST_ROOT}/cli-lock.json")" != "0.61.0" ]]; then
        fail_test "CLI lock did not record the release version"
        return
    fi
    if [[ "$(jq '.artifacts | length' "${TEST_ROOT}/cli-lock.json")" != "2" ]]; then
        fail_test "CLI lock did not contain every expected artifact"
        return
    fi
    if [[ "$(grep -c '^oras push ' "${TEST_ROOT}/calls")" != "2" ]]; then
        fail_test "stage did not push every CLI artifact"
        return
    fi
    if grep -Eq "oras push .* ${TEST_ROOT}/" "${TEST_ROOT}/calls" \
                                                                  || ! grep -Fq ' ./rad ' "${TEST_ROOT}/calls" \
                                                  || ! grep -Fq ' ./rad.exe ' "${TEST_ROOT}/calls"; then
        fail_test "CLI artifacts were not pushed from basename-relative paths"
        return
    fi
    ((++PASS))
}

test_real_oras_preserves_basename() {
    local real_oras
    local layout
    local pull_dir
    local release_dir

    setup_fixture
    layout="${TEST_ROOT}/layout"
    pull_dir="${TEST_ROOT}/pulled"
    release_dir="${TEST_ROOT}/release-cli"
    mkdir -p "${release_dir}"
    cp "${TEST_ROOT}/dist/linux/rad" \
        "${release_dir}/rad_linux_amd64"
    cp "${TEST_ROOT}/dist/windows/rad.exe" \
        "${release_dir}/rad_windows_amd64.exe"
    real_oras="$(command -v oras)"
    mv "${TEST_ROOT}/bin/oras" "${TEST_ROOT}/bin/oras.fake"
    cat > "${TEST_ROOT}/bin/oras" << 'EOF'
#!/bin/bash
set -euo pipefail
command="$1"
shift
exec "${REAL_ORAS}" "${command}" --oci-layout "$@"
EOF
    chmod +x "${TEST_ROOT}/bin/oras"

    PATH="${TEST_ROOT}/bin:${PATH}" \
        REAL_ORAS="${real_oras}" \
        GORELEASER_PARITY_TARGETS="${TEST_ROOT}/targets.json" \
        bash "${SCRIPT}" stage-cli \
            --registry "${layout}" \
            --version 0.61.0 \
            --artifacts-dir "${release_dir}" \
            --output "${TEST_ROOT}/layout-lock.json" > /dev/null
    "${real_oras}" pull --oci-layout \
        "${layout}/rad/linux-amd64:0.61.0" \
        --output "${pull_dir}" > /dev/null
    if [[ ! -f "${pull_dir}/rad" ]] \
                                    || find "${pull_dir}" -mindepth 2 -type f | grep -q .; then
        fail_test "real ORAS push did not preserve the CLI basename"
        return
    fi
    ((++PASS))
}

test_stages_downloaded_release_binaries() {
    local release_dir

    setup_fixture
    release_dir="${TEST_ROOT}/release-cli"
    mkdir -p "${release_dir}"
    cp "${TEST_ROOT}/dist/linux/rad" \
        "${release_dir}/rad_linux_amd64"
    cp "${TEST_ROOT}/dist/windows/rad.exe" \
        "${release_dir}/rad_windows_amd64.exe"
    run_script stage-cli \
        --registry example.test/radius \
        --version 0.61.0 \
        --artifacts-dir "${release_dir}" \
        --output "${TEST_ROOT}/cli-lock.json"
    if [[ "$(jq '.artifacts | length' "${TEST_ROOT}/cli-lock.json")" != "2" ]]; then
        fail_test "downloaded release binaries were not staged"
        return
    fi
    ((++PASS))
}

test_promotes_stable_aliases() {
    setup_fixture
    stage_cli
    write_image_lock
    run_script promote \
        --version 0.61.0 \
        --channel 0.61 \
        --image-lock "${TEST_ROOT}/image-lock.json" \
        --cli-lock "${TEST_ROOT}/cli-lock.json"

    if [[ "$(grep -c '^docker buildx imagetools create ' \
        "${TEST_ROOT}/calls")" != "2" ]]; then
        fail_test "promotion did not alias every production image"
        return
    fi
    if [[ "$(grep -c '^oras tag ' "${TEST_ROOT}/calls")" != "2" ]]; then
        fail_test "promotion did not alias every CLI artifact"
        return
    fi
    if ! grep -Fq ' 0.61 latest' "${TEST_ROOT}/calls"; then
        fail_test "promotion did not create channel and latest aliases"
        return
    fi
    ((++PASS))
}

test_rejects_prerelease_promotion() {
    setup_fixture
    stage_cli
    write_image_lock
    if run_script promote \
        --version 0.61.0-rc.1 \
        --channel 0.61 \
        --image-lock "${TEST_ROOT}/image-lock.json" \
        --cli-lock "${TEST_ROOT}/cli-lock.json" > /dev/null 2>&1; then
        fail_test "prerelease promotion should fail closed"
        return
    fi
    ((++PASS))
}

test_detects_alias_divergence() {
    setup_fixture
    stage_cli
    write_image_lock
    if PATH="${TEST_ROOT}/bin:${PATH}" \
        FAKE_REGISTRY_STATE="${TEST_ROOT}/registry-state" \
        FAKE_REGISTRY_CALLS="${TEST_ROOT}/calls" \
        FAKE_CORRUPT_ALIAS=latest \
        GORELEASER_PARITY_TARGETS="${TEST_ROOT}/targets.json" \
        bash "${SCRIPT}" promote \
            --version 0.61.0 \
            --channel 0.61 \
            --image-lock "${TEST_ROOT}/image-lock.json" \
            --cli-lock "${TEST_ROOT}/cli-lock.json" > /dev/null 2>&1; then
        fail_test "promotion should reject a divergent alias"
        return
    fi
    ((++PASS))
}

test_detects_version_tag_divergence() {
    setup_fixture
    stage_cli
    write_image_lock
    if PATH="${TEST_ROOT}/bin:${PATH}" \
        FAKE_REGISTRY_STATE="${TEST_ROOT}/registry-state" \
        FAKE_REGISTRY_CALLS="${TEST_ROOT}/calls" \
        FAKE_CORRUPT_REFERENCE=example.test/radius/ucpd:0.61.0 \
        GORELEASER_PARITY_TARGETS="${TEST_ROOT}/targets.json" \
        bash "${SCRIPT}" promote \
            --version 0.61.0 \
            --channel 0.61 \
            --image-lock "${TEST_ROOT}/image-lock.json" \
            --cli-lock "${TEST_ROOT}/cli-lock.json" > /dev/null 2>&1; then
        fail_test "promotion should reject a divergent full-version tag"
        return
    fi
    if grep -Eq '^(oras tag|docker buildx imagetools create)' \
        "${TEST_ROOT}/calls"; then
        fail_test "promotion mutated aliases before verifying every version tag"
        return
    fi
    ((++PASS))
}

test_rejects_lock_from_another_source() {
    setup_fixture
    stage_cli
    write_image_lock
    jq '.sourceSha = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"' \
        "${TEST_ROOT}/cli-lock.json" > "${TEST_ROOT}/wrong-cli-lock.json"
    if run_script verify \
        --version 0.61.0 \
        --image-lock "${TEST_ROOT}/image-lock.json" \
        --cli-lock "${TEST_ROOT}/wrong-cli-lock.json" > /dev/null 2>&1; then
        fail_test "lock from another source commit should be rejected"
        return
    fi
    ((++PASS))
}

test_retries_transient_registry_failures() {
    setup_fixture
    PATH="${TEST_ROOT}/bin:${PATH}" \
        FAKE_REGISTRY_STATE="${TEST_ROOT}/registry-state" \
        FAKE_REGISTRY_CALLS="${TEST_ROOT}/calls" \
        FAKE_FAIL_PUSH_ONCE=true \
        FAKE_FAIL_PULL_ONCE=true \
        RELEASE_RETRY_NO_SLEEP=true \
        GORELEASER_PARITY_TARGETS="${TEST_ROOT}/targets.json" \
        bash "${SCRIPT}" stage-cli \
            --registry example.test/radius \
            --version 0.61.0 \
            --artifacts "${TEST_ROOT}/artifacts.json" \
            --output "${TEST_ROOT}/cli-lock.json" > /dev/null
    write_image_lock
    PATH="${TEST_ROOT}/bin:${PATH}" \
        FAKE_REGISTRY_STATE="${TEST_ROOT}/registry-state" \
        FAKE_REGISTRY_CALLS="${TEST_ROOT}/calls" \
        FAKE_FAIL_TAG_ONCE=true \
        FAKE_FAIL_IMAGE_ALIAS_ONCE=true \
        RELEASE_RETRY_NO_SLEEP=true \
        GORELEASER_PARITY_TARGETS="${TEST_ROOT}/targets.json" \
        bash "${SCRIPT}" promote \
            --version 0.61.0 \
            --channel 0.61 \
            --image-lock "${TEST_ROOT}/image-lock.json" \
            --cli-lock "${TEST_ROOT}/cli-lock.json" > /dev/null

    if [[ "$(grep -c '^oras push ' "${TEST_ROOT}/calls")" != "3" ]]; then
        fail_test "CLI publication did not retry one transient failure"
        return
    fi
    if [[ "$(grep -c '^oras tag ' "${TEST_ROOT}/calls")" != "3" ]]; then
        fail_test "CLI alias promotion did not retry one transient failure"
        return
    fi
    if [[ "$(grep -c '^docker buildx imagetools create ' \
        "${TEST_ROOT}/calls")" != "3" ]]; then
        fail_test "image alias promotion did not retry one transient failure"
        return
    fi
    ((++PASS))
}

test_rejects_stale_cli_tag_without_overwriting() {
    local stale
    local digest
    local blob

    setup_fixture
    stale="${TEST_ROOT}/stale-rad"
    printf 'stale binary\n' > "${stale}"
    digest="sha256:$(digest_for stale)"
    mkdir -p "${TEST_ROOT}/registry-state.blobs"
    blob="${TEST_ROOT}/registry-state.blobs/${digest#sha256:}"
    cp "${stale}" "${blob}"
    printf '%s\t%s\t%s\t%s\n' \
        example.test/radius/rad/linux-amd64:0.61.0 \
        "${digest}" "${blob}" rad >> "${TEST_ROOT}/registry-state"

    if run_script stage-cli \
        --registry example.test/radius \
        --version 0.61.0 \
        --artifacts "${TEST_ROOT}/artifacts.json" \
        --output "${TEST_ROOT}/cli-lock.json" > /dev/null 2>&1; then
        fail_test "stale immutable CLI tag should be rejected"
        return
    fi
    if grep -Fq \
        'oras push example.test/radius/rad/linux-amd64:0.61.0' \
        "${TEST_ROOT}/calls"; then
        fail_test "stale immutable CLI tag was overwritten"
        return
    fi
    ((++PASS))
}

test_requires_image_lock_before_reusing_version_tag() {
    setup_fixture
    run_script assert-images-absent \
        --registry example.test/radius \
        --version 0.61.0 \
        --categories production
    write_image_lock
    if run_script assert-images-absent \
        --registry example.test/radius \
        --version 0.61.0 \
        --categories production > /dev/null 2>&1; then
        fail_test "unlocked immutable image tag should block publication"
        return
    fi
    ((++PASS))
}

test_image_preflight_fails_closed_on_lookup_errors() {
    local error

    for error in silent credentials; do
        setup_fixture
        if PATH="${TEST_ROOT}/bin:${PATH}" \
            FAKE_REGISTRY_STATE="${TEST_ROOT}/registry-state" \
            FAKE_REGISTRY_CALLS="${TEST_ROOT}/calls" \
            FAKE_IMAGE_LOOKUP_ERROR="${error}" \
            RELEASE_RETRY_NO_SLEEP=true \
            GORELEASER_PARITY_TARGETS="${TEST_ROOT}/targets.json" \
            bash "${SCRIPT}" assert-images-absent \
                --registry example.test/radius \
                --version 0.61.0 \
                --categories production > /dev/null 2>&1; then
            fail_test "image preflight accepted ${error} lookup failure"
            return
        fi
    done
    ((++PASS))
}

main() {
    export RELEASE_SOURCE_SHA="${SOURCE_SHA}"
    test_stages_cli_artifacts
    test_real_oras_preserves_basename
    test_stages_downloaded_release_binaries
    test_promotes_stable_aliases
    test_rejects_prerelease_promotion
    test_detects_alias_divergence
    test_detects_version_tag_divergence
    test_rejects_lock_from_another_source
    test_retries_transient_registry_failures
    test_rejects_stale_cli_tag_without_overwriting
    test_requires_image_lock_before_reusing_version_tag
    test_image_preflight_fails_closed_on_lookup_errors

    if ((FAIL > 0)); then
        echo "release OCI artifact tests failed: ${PASS} passed, ${FAIL} failed"
        exit 1
    fi
    echo "release OCI artifact tests passed (${PASS} tests)"
}

main "$@"
