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
readonly SCRIPT="${SCRIPT_DIR}/release-attestations.sh"
readonly TARGETS="${SCRIPT_DIR}/../release-parity/targets.json"
readonly VERSION="0.63.0-rc.1"
readonly SOURCE_SHA="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
readonly CHART_DIGEST="sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
readonly POLICY="--repo radius-project/radius --signer-workflow radius-project/radius/.github/workflows/build-release.yaml --source-ref refs/tags/v${VERSION} --source-digest ${SOURCE_SHA} --deny-self-hosted-runners"

TEST_ROOT=""
ASSETS=""
STATUS=0
OUTPUT=""
PASS=0
FAIL=0

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

sha256() {
    if command -v sha256sum > /dev/null 2>&1; then
        sha256sum | cut -d ' ' -f 1
    else
        shasum -a 256 | cut -d ' ' -f 1
    fi
}

write_lock() {
    local output="$1"
    local name
    local digest

    shift
    for name in "$@"; do
        digest="sha256:$(printf '%s' "${name}" | sha256)"
        jq -n --arg name "${name}" --arg digest "${digest}" '{
            name: $name,
            digest: $digest,
            immutableReference: ("ghcr.io/radius-project/" + $name + "@" + $digest)
        }'
    done | jq -s '.' > "${output}"
}

setup_release() {
    local name
    local -a production=()

    ASSETS="${TEST_ROOT}/assets"
    rm -rf "${ASSETS}" "${TEST_ROOT}/gh.log" "${TEST_ROOT}/gh.log.transient"
    mkdir -p "${ASSETS}"
    while IFS= read -r name; do
        printf 'binary %s\n' "${name}" > "${ASSETS}/${name}"
        printf '%s *%s\n' "$(sha256 < "${ASSETS}/${name}")" "${name}" \
            > "${ASSETS}/${name}.sha256"
    done < <(jq -r '.cliAssets[].name' "${TARGETS}")
    while IFS= read -r name; do
        production+=("${name}")
    done < <(jq -r '.images[]
        | select(.radiusBuild and .category == "production") | .name' \
        "${TARGETS}")
    write_lock "${ASSETS}/production-image-digests.json" "${production[@]}"
    while IFS= read -r name; do
        write_lock "${ASSETS}/${name}-image-digests.json" "${name}"
    done < <(jq -r '.images[]
        | select(.radiusBuild and .category != "production") | .name' \
        "${TARGETS}")
}

write_fake_gh() {
    mkdir -p "${TEST_ROOT}/bin"
    cat > "${TEST_ROOT}/bin/gh" << 'EOF'
#!/bin/bash
set -euo pipefail
printf '%s\n' "$*" >> "${FAKE_GH_LOG}"
case "${FAKE_GH_MODE}" in
    reject)
        if [[ "$3" == *ghcr.io/radius-project/ucpd@* ]]; then
            echo "Verification failed: no attestation matched the policy" >&2
            exit 1
        fi
        ;;
    missing)
        # Real gh output includes digests, which can contain 429 or 5xx digits.
        echo "Error: no attestations for sha256:5004290000: HTTP 404: Not Found" >&2
        exit 1
        ;;
    transient)
        if [[ ! -f "${FAKE_GH_LOG}.transient" ]]; then
            : > "${FAKE_GH_LOG}.transient"
            echo "HTTP 502: Bad Gateway" >&2
            exit 1
        fi
        ;;
esac
echo "Verification succeeded"
EOF
    chmod +x "${TEST_ROOT}/bin/gh"
}

run_script() {
    local gh_mode="$1"

    shift
    STATUS=0
    OUTPUT="$(
        PATH="${TEST_ROOT}/bin:${PATH}" FAKE_GH_MODE="${gh_mode}" \
            FAKE_GH_LOG="${TEST_ROOT}/gh.log" RELEASE_RETRY_ATTEMPTS=3 \
            RELEASE_RETRY_NO_SLEEP=true bash "${SCRIPT}" "$@" 2>&1
    )" || STATUS=$?
}

run_subjects() {
    run_script ok subjects --version "${VERSION}" --targets "${TARGETS}" \
        --assets-dir "${ASSETS}" --chart-digest "$1" \
        --output "${TEST_ROOT}/subjects.txt"
}

run_verify() {
    run_script "$1" verify --version "${VERSION}" \
        --source-sha "${2:-${SOURCE_SHA}}" --targets "${TARGETS}" \
        --assets-dir "${ASSETS}" --chart-digest "${CHART_DIGEST}"
}

subject_count() {
    jq '(.cliAssets | length) + ([.images[] | select(.radiusBuild)] | length)
        + 1' "${TARGETS}"
}

gh_calls() {
    if [[ -f "${TEST_ROOT}/gh.log" ]]; then
        wc -l < "${TEST_ROOT}/gh.log" | tr -d ' '
    else
        echo 0
    fi
}

test_subjects_cover_every_release_output() {
    local subjects="${TEST_ROOT}/subjects.txt"
    local name

    setup_release
    run_subjects "${CHART_DIGEST}"
    if [[ "${STATUS}" -ne 0 ]]; then
        fail_test "complete release subjects were rejected: ${OUTPUT}"
        return
    fi
    if [[ "$(wc -l < "${subjects}" | tr -d ' ')" != "$(subject_count)" ]] \
        || grep -Evq '^[0-9a-f]{64}  [^ ]+$' "${subjects}"; then
        fail_test "subjects are incomplete or not in checksums format"
        return
    fi
    if ! grep -Fqx "$(sha256 < "${ASSETS}/rad_windows_arm64.exe")  rad_windows_arm64.exe" \
        "${subjects}" || ! grep -Fqx \
        "${CHART_DIGEST#sha256:}  ghcr.io/radius-project/helm-chart/radius" \
        "${subjects}"; then
        fail_test "CLI or Helm chart subject does not match its staged digest"
        return
    fi
    while IFS= read -r name; do
        if ! grep -Fqx "$(printf '%s' "${name}" | sha256)  ghcr.io/radius-project/${name}" \
            "${subjects}"; then
            fail_test "Radius-built image ${name} is not attested"
            return
        fi
    done < <(jq -r '.images[] | select(.radiusBuild) | .name' "${TARGETS}")
    if grep -Eq 'dashboard|deployment-engine' "${subjects}"; then
        fail_test "externally built images are attested by Radius"
        return
    fi
    ((++PASS))
}

test_subjects_reject_inconsistent_evidence() {
    local name
    local chart
    local lock="production-image-digests.json"

    for name in unnormalized wrong-asset missing-lock missing-image \
        moved-image chart-digest; do
        setup_release
        chart="${CHART_DIGEST}"
        case "${name}" in
            unnormalized)
                printf '%s  rad_linux_amd64\n' \
                    "$(sha256 < "${ASSETS}/rad_linux_amd64")" \
                    > "${ASSETS}/rad_linux_amd64.sha256"
                ;;
            wrong-asset)
                printf '%s *rad_linux_arm64\n' \
                    "$(sha256 < "${ASSETS}/rad_linux_amd64")" \
                    > "${ASSETS}/rad_linux_amd64.sha256"
                ;;
            missing-lock) rm "${ASSETS}/bicep-image-digests.json" ;;
            missing-image)
                jq 'del(.[0])' "${ASSETS}/${lock}" > "${ASSETS}/lock.json"
                mv "${ASSETS}/lock.json" "${ASSETS}/${lock}"
                ;;
            moved-image)
                jq '.[0].immutableReference |= sub("radius-project"; "fork")' \
                    "${ASSETS}/${lock}" > "${ASSETS}/lock.json"
                mv "${ASSETS}/lock.json" "${ASSETS}/${lock}"
                ;;
            chart-digest) chart="sha256:abc" ;;
        esac
        run_subjects "${chart}"
        if [[ "${STATUS}" -eq 0 ]]; then
            fail_test "${name} release evidence produced attestation subjects"
            return
        fi
    done
    ((++PASS))
}

test_verify_enforces_release_policy() {
    setup_release
    run_verify ok
    if [[ "${STATUS}" -ne 0 ]]; then
        fail_test "complete release provenance was rejected: ${OUTPUT}"
        return
    fi
    if [[ "$(gh_calls)" != "$(subject_count)" ]] \
        || grep -Fvq -e "${POLICY}" "${TEST_ROOT}/gh.log"; then
        fail_test "a subject was skipped or verified without the release policy"
        return
    fi
    if ! grep -Fq "attestation verify ${ASSETS}/rad_linux_amd64 --repo" \
        "${TEST_ROOT}/gh.log" || ! grep -Fq \
        "attestation verify oci://ghcr.io/radius-project/bicep@sha256:" \
        "${TEST_ROOT}/gh.log" || ! grep -Fq \
        "attestation verify oci://ghcr.io/radius-project/helm-chart/radius@${CHART_DIGEST} --repo" \
        "${TEST_ROOT}/gh.log"; then
        fail_test "verification did not use the staged files and locked digests"
        return
    fi
    ((++PASS))
}

test_verify_fails_closed() {
    setup_release
    run_verify reject
    if [[ "${STATUS}" -eq 0 ]] \
        || [[ "${OUTPUT}" != *"verification failed for ghcr.io/radius-project/ucpd"* ]]; then
        fail_test "an image without matching provenance was accepted"
        return
    fi

    setup_release
    run_verify missing
    if [[ "${STATUS}" -eq 0 || "$(gh_calls)" != "1" ]]; then
        fail_test "missing provenance was accepted or retried"
        return
    fi

    setup_release
    rm "${ASSETS}/rad_linux_arm64"
    run_verify ok
    if [[ "${STATUS}" -eq 0 ]] \
        || [[ "${OUTPUT}" != *"release asset not found"* ]]; then
        fail_test "a missing staged CLI asset was accepted"
        return
    fi

    setup_release
    run_verify ok "${SOURCE_SHA:0:12}"
    if [[ "${STATUS}" -eq 0 || "$(gh_calls)" != "0" ]]; then
        fail_test "verification ran without the full source commit"
        return
    fi
    ((++PASS))
}

test_verify_retries_transient_errors() {
    setup_release
    run_verify transient
    if [[ "${STATUS}" -ne 0 ]] \
        || [[ "$(gh_calls)" != "$(($(subject_count) + 1))" ]]; then
        fail_test "a transient provenance lookup failure was not retried"
        return
    fi
    ((++PASS))
}

main() {
    command -v jq > /dev/null

    TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/release-attestations-test-XXXXXX")"
    write_fake_gh

    test_subjects_cover_every_release_output
    test_subjects_reject_inconsistent_evidence
    test_verify_enforces_release_policy
    test_verify_fails_closed
    test_verify_retries_transient_errors

    if ((FAIL > 0)); then
        echo "Release attestation tests failed: ${PASS} passed, ${FAIL} failed"
        exit 1
    fi
    echo "Release attestation tests passed (${PASS} tests)"
}

main "$@"
