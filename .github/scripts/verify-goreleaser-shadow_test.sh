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
readonly SCRIPT="${SCRIPT_DIR}/verify-goreleaser-shadow.sh"
readonly VERSION="0.60.0"
readonly CHANNEL="0.60"
readonly SOURCE_COMMIT="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
readonly SHADOW_INDEX="sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
readonly PRODUCTION_INDEX="sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"

TEST_ROOT=""
PASS=0
FAIL=0
LAST_OUTPUT=""
LAST_STATUS=0

cleanup() {
    if [[ -n "${TEST_ROOT}" && -d "${TEST_ROOT}" ]]; then
        rm -rf "${TEST_ROOT}"
    fi
}
trap cleanup EXIT

sha256_file() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1" | awk '{print $1}'
    else
        shasum -a 256 "$1" | awk '{print $1}'
    fi
}

setup_fixture() {
    cleanup
    TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/goreleaser-shadow-test-XXXXXX")"
    mkdir -p \
        "${TEST_ROOT}/shadow/rad_linux_amd64_v1" \
        "${TEST_ROOT}/production" \
        "${TEST_ROOT}/baselines" \
        "${TEST_ROOT}/build-info" \
        "${TEST_ROOT}/shadow-images" \
        "${TEST_ROOT}/production-images" \
        "${TEST_ROOT}/shadow-payload" \
        "${TEST_ROOT}/production-payload"

    printf 'identical-rad-binary' \
        >"${TEST_ROOT}/shadow/rad_linux_amd64_v1/rad"
    cp "${TEST_ROOT}/shadow/rad_linux_amd64_v1/rad" \
        "${TEST_ROOT}/production/rad_linux_amd64"
    sha256_file "${TEST_ROOT}/shadow/rad_linux_amd64_v1/rad" \
        >"${TEST_ROOT}/shadow/rad_linux_amd64.sha256"

    cat >"${TEST_ROOT}/targets.json" <<'EOF'
{
  "cliAssets": [
    {"name": "rad_linux_amd64", "os": "linux", "arch": "amd64"}
  ],
  "images": [
    {
      "category": "production",
      "name": "ucpd",
      "radiusBuild": true,
    "requiredPlatforms": ["linux/amd64", "linux/arm64"]
    }
  ]
}
EOF

    cat >"${TEST_ROOT}/baselines/v0.60.0.json" <<'EOF'
{
  "release": {"version": "0.60.0", "prerelease": false},
    "cli": {
        "assets": [
            {
                "name": "rad_linux_amd64",
                "build": {
                    "path": "github.com/radius-project/radius/cmd/rad",
                    "module": {"path": "github.com/radius-project/radius"},
                    "settings": {
                        "-buildmode": "exe",
                        "-compiler": "gc",
                        "CGO_ENABLED": "0",
                        "GOOS": "linux",
                        "GOARCH": "amd64",
                        "GOAMD64": "v1"
                    }
                }
            }
        ]
    },
  "images": [
    {
      "category": "production",
      "name": "ucpd",
            "platforms": [
                {
                    "platform": "linux/amd64",
                    "config": {
                        "user": "65532:65532",
                        "entrypoint": ["/ucpd"],
                        "command": [],
                        "workingDirectory": "/",
                        "environment": ["PATH=/usr/bin"],
                        "labels": {
                            "org.opencontainers.image.description": "ucpd",
                            "org.opencontainers.image.revision": "old-commit",
                            "org.opencontainers.image.source": "https://github.com/radius-project/radius",
                            "org.opencontainers.image.version": "0.60.0"
                        }
                    }
                },
                {
                    "platform": "linux/arm64",
                    "config": {
                        "user": "65532:65532",
                        "entrypoint": ["/ucpd"],
                        "command": [],
                        "workingDirectory": "/",
                        "environment": ["PATH=/usr/bin"],
                        "labels": {
                            "org.opencontainers.image.description": "ucpd",
                            "org.opencontainers.image.revision": "old-commit",
                            "org.opencontainers.image.source": "https://github.com/radius-project/radius",
                            "org.opencontainers.image.version": "0.60.0"
                        }
                    }
                }
            ],
            "auxiliaryManifests": []
    }
  ]
}
EOF

    cat >"${TEST_ROOT}/shadow/artifacts.json" <<'EOF'
[
  {
    "name": "rad_linux_amd64",
    "path": "dist/goreleaser/rad_linux_amd64_v1/rad",
    "type": "Binary",
    "extra": {"ID": "rad"}
  }
]
EOF

    cat >"${TEST_ROOT}/build-info/shadow-rad_linux_amd64.json" <<EOF
{
  "goVersion": "go1.26.5",
  "path": "github.com/radius-project/radius/cmd/rad",
  "module": {"Path": "github.com/radius-project/radius"},
    "settings": {
        "-buildmode": "exe",
        "-compiler": "gc",
        "CGO_ENABLED": "0",
        "GOOS": "linux",
        "GOARCH": "amd64",
        "GOAMD64": "v1"
    },
  "linkerMetadata": {
    "channel": "${CHANNEL}",
    "release": "${VERSION}",
    "commit": "${SOURCE_COMMIT}",
    "version": "v${VERSION}",
    "chartVersion": "${VERSION}",
    "terraformVersion": "1.15.8"
  }
}
EOF
    cp "${TEST_ROOT}/build-info/shadow-rad_linux_amd64.json" \
        "${TEST_ROOT}/build-info/production-rad_linux_amd64.json"

    cat >"${TEST_ROOT}/build-info/shadow-runtime.json" <<EOF
{"release":"${VERSION}","version":"v${VERSION}","commit":"${SOURCE_COMMIT}"}
EOF
    cp "${TEST_ROOT}/build-info/shadow-runtime.json" \
        "${TEST_ROOT}/build-info/production-runtime.json"

    write_image_fixture "${TEST_ROOT}/shadow-images/ucpd.json" \
        "${SHADOW_INDEX}"
    write_image_fixture "${TEST_ROOT}/production-images/ucpd.json" \
        "${PRODUCTION_INDEX}"
    cat >"${TEST_ROOT}/production-image-digests.json" <<EOF
[
    {
        "name": "ucpd",
        "reference": "ghcr.io/radius-project/ucpd:${CHANNEL}",
        "digest": "${PRODUCTION_INDEX}",
        "immutableReference": "ghcr.io/radius-project/ucpd@${PRODUCTION_INDEX}",
        "platforms": ["linux/amd64", "linux/arm64"]
    }
]
EOF

    local platform_slug
    for platform_slug in linux_amd64 linux_arm64; do
        cat >"${TEST_ROOT}/shadow-payload/ucpd-${platform_slug}.payload.jsonl" <<'EOF'
{"gid":0,"mode":493,"path":"manifest","type":"directory","uid":0}
{"gid":0,"mode":420,"path":"manifest/built-in-providers/radius_core.yaml","sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","size":10,"type":"file","uid":0}
{"gid":65532,"mode":493,"path":"ucpd","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","size":13,"type":"file","uid":65532}
EOF
        cp "${TEST_ROOT}/shadow-payload/ucpd-${platform_slug}.payload.jsonl" \
            "${TEST_ROOT}/production-payload/ucpd-${platform_slug}.payload.jsonl"
        printf '%s\n' "$(printf 'server-%s' "${platform_slug}" | sha256_stdin)" \
            >"${TEST_ROOT}/shadow-payload/ucpd-${platform_slug}.binary.sha256"
        cp "${TEST_ROOT}/shadow-payload/ucpd-${platform_slug}.binary.sha256" \
            "${TEST_ROOT}/production-payload/ucpd-${platform_slug}.binary.sha256"
    done
}

sha256_stdin() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum | awk '{print $1}'
    else
        shasum -a 256 | awk '{print $1}'
    fi
}

write_image_fixture() {
    local output="$1"
    local digest="$2"

    cat >"${output}" <<EOF
{
  "manifest": {
    "mediaType": "application/vnd.oci.image.index.v1+json",
    "digest": "${digest}",
    "manifests": [
      {
        "mediaType": "application/vnd.oci.image.manifest.v1+json",
        "digest": "sha256:amd64",
        "platform": {"architecture": "amd64", "os": "linux"}
            },
            {
                "mediaType": "application/vnd.oci.image.manifest.v1+json",
                "digest": "sha256:arm64",
                "platform": {"architecture": "arm64", "os": "linux"}
      }
    ]
  },
  "image": {
    "linux/amd64": {
      "architecture": "amd64",
      "os": "linux",
      "config": {
        "User": "65532:65532",
        "Entrypoint": ["/ucpd"],
        "Cmd": [],
        "WorkingDir": "/",
        "Env": ["PATH=/usr/bin"],
        "Labels": {
          "org.opencontainers.image.description": "ucpd",
          "org.opencontainers.image.revision": "${SOURCE_COMMIT}",
          "org.opencontainers.image.source": "https://github.com/radius-project/radius",
          "org.opencontainers.image.version": "${VERSION}"
        }
      }
        },
        "linux/arm64": {
            "architecture": "arm64",
            "os": "linux",
            "config": {
                "User": "65532:65532",
                "Entrypoint": ["/ucpd"],
                "Cmd": [],
                "WorkingDir": "/",
                "Env": ["PATH=/usr/bin"],
                "Labels": {
                    "org.opencontainers.image.description": "ucpd",
                    "org.opencontainers.image.revision": "${SOURCE_COMMIT}",
                    "org.opencontainers.image.source": "https://github.com/radius-project/radius",
                    "org.opencontainers.image.version": "${VERSION}"
                }
            }
    }
  }
}
EOF
}

run_verifier() {
    LAST_STATUS=0
    LAST_OUTPUT="$(
        GORELEASER_PARITY_TARGETS="${TEST_ROOT}/targets.json" \
            GORELEASER_PARITY_BASELINES="${TEST_ROOT}/baselines" \
            GORELEASER_BUILD_INFO_DIR="${TEST_ROOT}/build-info" \
            GORELEASER_SHADOW_IMAGE_DATA_DIR="${TEST_ROOT}/shadow-images" \
            GORELEASER_PRODUCTION_IMAGE_DATA_DIR="${TEST_ROOT}/production-images" \
            GORELEASER_SHADOW_PAYLOAD_DIR="${TEST_ROOT}/shadow-payload" \
            GORELEASER_PRODUCTION_PAYLOAD_DIR="${TEST_ROOT}/production-payload" \
            GORELEASER_PRODUCTION_IMAGE_LOCK="${TEST_ROOT}/production-image-digests.json" \
            bash "${SCRIPT}" \
            --shadow-dir "${TEST_ROOT}/shadow" \
            --production-dir "${TEST_ROOT}/production" \
            --version "${VERSION}" \
            --channel "${CHANNEL}" \
            --chart-version "${VERSION}" \
            --commit "${SOURCE_COMMIT}" \
            --report "${TEST_ROOT}/report.json" 2>&1
    )" || LAST_STATUS=$?
}

pass() {
    ((++PASS))
}

fail_test() {
    echo "  ASSERT FAILED: $1"
    echo "  Output: ${LAST_OUTPUT}"
    ((++FAIL))
}

test_matching_outputs_pass() {
    setup_fixture
    run_verifier

    if [[ "${LAST_STATUS}" -ne 0 ]]; then
        fail_test "matching outputs should pass"
        return
    fi
    jq -e '
        .checks.cliBinaryDigests == "match"
        and .checks.imageRuntimeConfiguration == "match"
        and (.knownDifferences | length) == 1
    ' "${TEST_ROOT}/report.json" >/dev/null || {
        fail_test "parity report is incomplete"
        return
    }
    pass
}

test_binary_mismatch_fails() {
    setup_fixture
    printf 'different-production-binary' \
        >"${TEST_ROOT}/production/rad_linux_amd64"
    run_verifier

    if [[ "${LAST_STATUS}" -eq 0 ||
        "${LAST_OUTPUT}" != *"binary digest mismatch"* ]]; then
        fail_test "binary mismatch should fail"
        return
    fi
    pass
}

test_image_config_mismatch_fails() {
    setup_fixture
    jq '.image["linux/arm64"].config.ExposedPorts = {"9000/tcp": {}}' \
        "${TEST_ROOT}/shadow-images/ucpd.json" \
        >"${TEST_ROOT}/shadow-images/ucpd.tmp"
    mv "${TEST_ROOT}/shadow-images/ucpd.tmp" \
        "${TEST_ROOT}/shadow-images/ucpd.json"
    run_verifier

    if [[ "${LAST_STATUS}" -eq 0 ||
        "${LAST_OUTPUT}" != *"runtime image configuration"* ]]; then
        fail_test "image configuration mismatch should fail"
        return
    fi
    pass
}

test_payload_mismatch_fails() {
    setup_fixture
    echo '{"path":"unexpected-file","type":"file"}' \
        >>"${TEST_ROOT}/shadow-payload/ucpd-linux_arm64.payload.jsonl"
    run_verifier

    if [[ "${LAST_STATUS}" -eq 0 ||
        "${LAST_OUTPUT}" != *"runtime payload mismatch"* ]]; then
        fail_test "payload mismatch should fail"
        return
    fi
    pass
}

test_checksum_mismatch_fails() {
    setup_fixture
    printf '%064d\n' 0 >"${TEST_ROOT}/shadow/rad_linux_amd64.sha256"
    run_verifier

    if [[ "${LAST_STATUS}" -eq 0 ||
        "${LAST_OUTPUT}" != *"shadow checksum mismatch"* ]]; then
        fail_test "checksum mismatch should fail"
        return
    fi
    pass
}

test_server_binary_mismatch_fails() {
    setup_fixture
    printf '%064d\n' 0 \
        >"${TEST_ROOT}/shadow-payload/ucpd-linux_arm64.binary.sha256"
    run_verifier

    if [[ "${LAST_STATUS}" -eq 0 ||
        "${LAST_OUTPUT}" != *"embedded server binary mismatch"* ]]; then
        fail_test "server binary mismatch should fail"
        return
    fi
    pass
}

test_baseline_contract_mismatch_fails() {
    setup_fixture
    jq '.images[0].platforms = [{"platform":"linux/amd64"}]' \
        "${TEST_ROOT}/baselines/v0.60.0.json" \
        >"${TEST_ROOT}/baselines/v0.60.0.tmp"
    mv "${TEST_ROOT}/baselines/v0.60.0.tmp" \
        "${TEST_ROOT}/baselines/v0.60.0.json"
    run_verifier

    if [[ "${LAST_STATUS}" -eq 0 ||
        "${LAST_OUTPUT}" != *"baseline and target production image sets"* ]]; then
        fail_test "baseline contract mismatch should fail"
        return
    fi
    pass
}

test_baseline_runtime_mismatch_fails() {
    setup_fixture
    jq '.images[0].platforms[1].config.user = "root"' \
        "${TEST_ROOT}/baselines/v0.60.0.json" \
        >"${TEST_ROOT}/baselines/v0.60.0.tmp"
    mv "${TEST_ROOT}/baselines/v0.60.0.tmp" \
        "${TEST_ROOT}/baselines/v0.60.0.json"
    run_verifier

    if [[ "${LAST_STATUS}" -eq 0 ||
        "${LAST_OUTPUT}" != *"baseline runtime image contract"* ]]; then
        fail_test "baseline runtime mismatch should fail"
        return
    fi
    pass
}

test_production_image_lock_mismatch_fails() {
    setup_fixture
    jq '.[0].digest = "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"' \
        "${TEST_ROOT}/production-image-digests.json" \
        >"${TEST_ROOT}/production-image-digests.tmp"
    mv "${TEST_ROOT}/production-image-digests.tmp" \
        "${TEST_ROOT}/production-image-digests.json"
    run_verifier

    if [[ "${LAST_STATUS}" -eq 0 ||
        "${LAST_OUTPUT}" != *"does not match the lock"* ]]; then
        fail_test "production image lock mismatch should fail"
        return
    fi
    pass
}

main() {
    test_matching_outputs_pass
    test_binary_mismatch_fails
    test_image_config_mismatch_fails
    test_payload_mismatch_fails
    test_checksum_mismatch_fails
    test_server_binary_mismatch_fails
    test_baseline_contract_mismatch_fails
    test_baseline_runtime_mismatch_fails
    test_production_image_lock_mismatch_fails

    if ((FAIL > 0)); then
        echo "GoReleaser shadow verifier tests failed: ${PASS} passed, ${FAIL} failed"
        exit 1
    fi
    echo "GoReleaser shadow verifier tests passed (${PASS} tests)"
}

main "$@"
