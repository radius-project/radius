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
readonly SCRIPT="${SCRIPT_DIR}/capture-release-image-digests.sh"
readonly DIGEST="sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
readonly SOURCE_SHA="bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

TEST_ROOT=""

cleanup() {
    if [[ -n "${TEST_ROOT}" && -d "${TEST_ROOT}" ]]; then
        rm -rf "${TEST_ROOT}"
    fi
}
trap cleanup EXIT

setup() {
    local source_sha="${1:-${SOURCE_SHA}}"
    TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/capture-image-digests-test-XXXXXX")"
    mkdir -p "${TEST_ROOT}/bin"
    cat > "${TEST_ROOT}/targets.json" << 'EOF'
{
  "images": [
    {
      "category": "production",
      "name": "ucpd",
      "radiusBuild": true,
      "requiredPlatforms": ["linux/amd64", "linux/arm/v7", "linux/arm64"]
    }
  ]
}
EOF
    cat > "${TEST_ROOT}/bin/docker" << EOF
#!/bin/bash
set -euo pipefail
printf '%s\n' "\$*" >"\${DOCKER_ARGS_FILE}"
if [[ "\${FAIL_ONCE:-}" == "true" && ! -e "\${DOCKER_ARGS_FILE}.failed" ]]; then
  touch "\${DOCKER_ARGS_FILE}.failed"
  echo "503 Service Unavailable" >&2
  exit 1
fi
if [[ "\${MISSING_IMAGE_NAME:-}" != "" &&
  "\$*" == *"/\${MISSING_IMAGE_NAME}:"* ]]; then
  echo "manifest not found" >&2
  exit 1
fi
cat <<'JSON'
{
  "manifest": {
    "digest": "${DIGEST}",
    "manifests": [
      {"platform":{"os":"linux","architecture":"amd64"}},
      {"platform":{"os":"linux","architecture":"arm","variant":"v7"}},
      {"platform":{"os":"linux","architecture":"arm64"}},
      {"platform":{"os":"unknown","architecture":"unknown"}}
    ]
  },
  "image": {
    "linux/amd64": {
      "config": {"Labels": {
        "org.opencontainers.image.revision": "${source_sha}",
        "org.opencontainers.image.version": "0.61"
      }}
    },
    "linux/arm/v7": {
      "config": {"Labels": {
        "org.opencontainers.image.revision": "${source_sha}",
        "org.opencontainers.image.version": "0.61"
      }}
    },
    "linux/arm64": {
      "config": {"Labels": {
        "org.opencontainers.image.revision": "${source_sha}",
        "org.opencontainers.image.version": "0.61"
      }}
    }
  }
}
JSON
EOF
    chmod +x "${TEST_ROOT}/bin/docker"
}

main() {
    local output

    setup
    output="${TEST_ROOT}/digests.json"
    PATH="${TEST_ROOT}/bin:${PATH}" \
        DOCKER_ARGS_FILE="${TEST_ROOT}/docker-args" \
        GORELEASER_PARITY_TARGETS="${TEST_ROOT}/targets.json" \
        bash "${SCRIPT}" \
        --registry ghcr.io/radius-project \
        --tag 0.61 \
        --categories production \
        --source-sha "${SOURCE_SHA}" \
        --output "${output}"

    [[ "$(cat "${TEST_ROOT}/docker-args")" == "buildx imagetools inspect --format {{json .}} ghcr.io/radius-project/ucpd:0.61" ]]

    jq -e --arg digest "${DIGEST}" '
        length == 1
        and .[0].name == "ucpd"
        and .[0].reference == "ghcr.io/radius-project/ucpd:0.61"
        and .[0].digest == $digest
        and .[0].sourceSha == $sourceSha
        and .[0].immutableReference
            == ("ghcr.io/radius-project/ucpd@" + $digest)
        and .[0].platforms
            == ["linux/amd64", "linux/arm/v7", "linux/arm64"]
    ' --arg sourceSha "${SOURCE_SHA}" "${output}" > /dev/null

    rm -f "${TEST_ROOT}/docker-args.failed"
    PATH="${TEST_ROOT}/bin:${PATH}" \
        DOCKER_ARGS_FILE="${TEST_ROOT}/docker-args" \
        FAIL_ONCE=true \
        RELEASE_RETRY_NO_SLEEP=true \
        GORELEASER_PARITY_TARGETS="${TEST_ROOT}/targets.json" \
        bash "${SCRIPT}" \
        --registry ghcr.io/radius-project \
        --tag 0.61 \
        --categories production \
        --source-sha "${SOURCE_SHA}" \
        --output "${TEST_ROOT}/retried.json" > /dev/null
    [[ -s "${TEST_ROOT}/retried.json" ]]

    PATH="${TEST_ROOT}/bin:${PATH}" \
        DOCKER_ARGS_FILE="${TEST_ROOT}/docker-args" \
        GORELEASER_PARITY_TARGETS="${TEST_ROOT}/targets.json" \
        bash "${SCRIPT}" \
        --registry ghcr.io/radius-project \
        --tag 0.61 \
        --names ucpd \
        --source-sha "${SOURCE_SHA}" \
        --output "${TEST_ROOT}/named.json" > /dev/null
    [[ "$(jq -r '.[0].name' "${TEST_ROOT}/named.json")" == "ucpd" ]]

    if PATH="${TEST_ROOT}/bin:${PATH}" \
        DOCKER_ARGS_FILE="${TEST_ROOT}/docker-args" \
        GORELEASER_PARITY_TARGETS="${TEST_ROOT}/targets.json" \
        bash "${SCRIPT}" \
        --registry ghcr.io/radius-project \
        --tag 0.61 \
        --categories missing \
          --source-sha "${SOURCE_SHA}" \
        --output "${TEST_ROOT}/empty.json" > /dev/null 2>&1; then
          echo "empty image category selection should fail" >&2
          exit 1
    fi

        PATH="${TEST_ROOT}/bin:${PATH}" \
        DOCKER_ARGS_FILE="${TEST_ROOT}/docker-args" \
        MISSING_IMAGE_NAME=ucpd \
        RELEASE_RETRY_NO_SLEEP=true \
        GORELEASER_PARITY_TARGETS="${TEST_ROOT}/targets.json" \
          bash "${SCRIPT}" \
          --registry ghcr.io/radius-project \
          --tag 0.61 \
          --categories production \
          --source-sha "${SOURCE_SHA}" \
          --allow-absent \
          --state-output "${TEST_ROOT}/absent-state.txt" \
          --output "${TEST_ROOT}/absent.json" > /dev/null
        [[ "$(< "${TEST_ROOT}/absent-state.txt")" == "absent" ]]
        [[ "$(jq 'length' "${TEST_ROOT}/absent.json")" == "0" ]]

        PATH="${TEST_ROOT}/bin:${PATH}" \
        DOCKER_ARGS_FILE="${TEST_ROOT}/docker-args" \
        GORELEASER_PARITY_TARGETS="${TEST_ROOT}/targets.json" \
          bash "${SCRIPT}" \
          --registry ghcr.io/radius-project \
          --tag 0.61 \
          --categories production \
          --source-sha "${SOURCE_SHA}" \
          --allow-absent \
          --state-output "${TEST_ROOT}/complete-state.txt" \
          --output "${TEST_ROOT}/complete.json" > /dev/null
        [[ "$(< "${TEST_ROOT}/complete-state.txt")" == "complete" ]]

        setup "cccccccccccccccccccccccccccccccccccccccc"
        if PATH="${TEST_ROOT}/bin:${PATH}" \
        DOCKER_ARGS_FILE="${TEST_ROOT}/docker-args" \
        GORELEASER_PARITY_TARGETS="${TEST_ROOT}/targets.json" \
          bash "${SCRIPT}" \
          --registry ghcr.io/radius-project \
          --tag 0.61 \
          --categories production \
          --source-sha "${SOURCE_SHA}" \
          --output "${TEST_ROOT}/wrong-source.json" > /dev/null 2>&1; then
          echo "collector accepted an image from another source" >&2
          exit 1
    fi

        setup
        jq '.images += [{
          "category":"production",
          "name":"controller",
          "radiusBuild":true,
          "requiredPlatforms":["linux/amd64","linux/arm/v7","linux/arm64"]
        }]' "${TEST_ROOT}/targets.json" > "${TEST_ROOT}/targets.tmp"
        mv "${TEST_ROOT}/targets.tmp" "${TEST_ROOT}/targets.json"
        PATH="${TEST_ROOT}/bin:${PATH}" \
        DOCKER_ARGS_FILE="${TEST_ROOT}/docker-args" \
        MISSING_IMAGE_NAME=controller \
        RELEASE_RETRY_NO_SLEEP=true \
        GORELEASER_PARITY_TARGETS="${TEST_ROOT}/targets.json" \
          bash "${SCRIPT}" \
          --registry ghcr.io/radius-project \
          --tag 0.61 \
          --categories production \
          --source-sha "${SOURCE_SHA}" \
          --allow-absent \
          --state-output "${TEST_ROOT}/partial-state.txt" \
          --output "${TEST_ROOT}/partial.json" > /dev/null
        [[ "$(< "${TEST_ROOT}/partial-state.txt")" == "partial" ]]

        if PATH="${TEST_ROOT}/bin:${PATH}" \
        DOCKER_ARGS_FILE="${TEST_ROOT}/docker-args" \
        MISSING_IMAGE_NAME=controller \
        RELEASE_RETRY_NO_SLEEP=true \
        GORELEASER_PARITY_TARGETS="${TEST_ROOT}/targets.json" \
          bash "${SCRIPT}" \
          --registry ghcr.io/radius-project \
          --tag 0.61 \
          --categories production \
          --source-sha "${SOURCE_SHA}" \
          --output "${TEST_ROOT}/strict.json" > /dev/null 2>&1; then
          echo "strict capture accepted a partial image set" >&2
          exit 1
    fi

        if PATH="${TEST_ROOT}/bin:${PATH}" \
        DOCKER_ARGS_FILE="${TEST_ROOT}/docker-args" \
        GORELEASER_PARITY_TARGETS="${TEST_ROOT}/targets.json" \
          bash "${SCRIPT}" \
          --registry ghcr.io/radius-project \
          --tag 0.61 \
          --categories production \
          --source-sha "${SOURCE_SHA}" \
          --allow-absent \
          --output "${TEST_ROOT}/unreported.json" > /dev/null 2>&1; then
          echo "absence tolerance was accepted without a state output" >&2
          exit 1
    fi

    echo "release image digest capture tests passed"
}

main "$@"
