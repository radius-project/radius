#!/usr/bin/env bash

# Copyright 2026 The Radius Authors.
# Licensed under the Apache License, Version 2.0.
# You may obtain a copy of the License at
# http://www.apache.org/licenses/LICENSE-2.0

# Exercise the real Make image recipes without a Docker daemon or Go builds.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TEST_DIR="$(mktemp -d)"
trap 'rm -rf "${TEST_DIR}"' EXIT

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

export CALL_LOG="${TEST_DIR}/calls"
mkdir -p "${TEST_DIR}/bin" \
    "${TEST_DIR}/deploy/manifest/built-in-providers/self-hosted"
printf '{}\n' >"${TEST_DIR}/deploy/manifest/built-in-providers/self-hosted/test.json"
images=(ucpd applications-rp dynamic-rp controller testrp magpiego bicep pre-upgrade)
for image in "${images[@]}"; do
    directory="deploy/images/${image}"
    case "${image}" in
    testrp | magpiego) directory="test/${image}" ;;
    esac
    mkdir -p "${TEST_DIR}/${directory}"
    cp "${ROOT}/${directory}/Dockerfile" "${TEST_DIR}/${directory}/Dockerfile"
done
cat >"${TEST_DIR}/fixtures.mk" <<'EOF'
build-%:
	@target="$*"; arch="$${target##*-}"; image="$${target%-linux-*}"; \
	printf 'compile %s %s\n' "$${image}" "$${arch}" >>"$(CALL_LOG)"; \
	directory="$(OUT_DIR)/linux_$${arch}/release"; \
	if [ "$${image}" = bicep ]; then \
		directory="$${directory}/bicep"; \
	fi; \
	mkdir -p "$${directory}"; \
	touch "$${directory}/$${image}"
EOF
cat >"${TEST_DIR}/bin/docker" <<'EOF'
#!/bin/bash
set -euo pipefail
[[ "$1" == build && "$2" == . ]] || exit 1
printf 'docker %s\n' "$*" >>"${CALL_LOG}"
platform="" image="" dockerfile=""
while [[ "$#" -gt 0 ]]; do
    case "$1" in
    --platform) platform="$2"; shift 2 ;;
    -t) image="$2"; shift 2 ;;
    -f) dockerfile="$2"; shift 2 ;;
    *) shift ;;
    esac
done
[[ "${platform}" == "linux/${EXPECTED_ARCH}" ]] || exit 1
image="${image##*/}"
image="${image%:test}"
directory="linux_${EXPECTED_ARCH}/release"
if [[ "${image}" == bicep ]]; then
    directory+="/bicep"
fi
[[ -f "${directory}/${image}" ]] || exit 1
grep -F 'ARG TARGETARCH' "${dockerfile}" >/dev/null
grep -F 'linux_${TARGETARCH:-amd64}/release/' "${dockerfile}" >/dev/null
if [[ "${image}" == ucpd ]]; then
    [[ -f manifest/built-in-providers/test.json ]] || exit 1
fi
EOF
chmod +x "${TEST_DIR}/bin/docker"
export PATH="${TEST_DIR}/bin:${PATH}"
unset MAKEFLAGS MFLAGS DOCKER_BUILD_ARCH
cd "${TEST_DIR}"
for selection in default amd64 arm64; do
    : >"${CALL_LOG}"
    export EXPECTED_ARCH="${selection}"
    args=()
    if [[ "${selection}" == default ]]; then
        export EXPECTED_ARCH=amd64
    else
        args+=("DOCKER_BUILD_ARCH=${selection}")
    fi
    if ! make --no-print-directory --jobs=1 \
        -f "${ROOT}/build/docker.mk" -f "${TEST_DIR}/fixtures.mk" \
        docker-build OUT_DIR=./dist DOCKER_REGISTRY=radius-local \
        DOCKER_TAG_VERSION=test "${args[@]}" >"${TEST_DIR}/output" 2>&1; then
        cat "${TEST_DIR}/output"
        fail "${selection} image build failed"
    fi
    [[ "$(grep -c '^compile ' "${CALL_LOG}")" == "${#images[@]}" ]] ||
        fail "${selection} did not compile every image"
    [[ "$(grep -c '^docker ' "${CALL_LOG}")" == "${#images[@]}" ]] ||
        fail "${selection} did not build every image"
    for image in "${images[@]}"; do
        grep -Fx "compile ${image} ${EXPECTED_ARCH}" "${CALL_LOG}" >/dev/null ||
            fail "${image} compiled for the wrong architecture"
        grep -F -- "--platform linux/${EXPECTED_ARCH} -t radius-local/${image}:test" \
            "${CALL_LOG}" >/dev/null ||
            fail "${image} used the wrong Docker platform"
    done
done

echo "Docker build architecture tests passed"
