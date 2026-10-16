#!/bin/bash

# Copyright 2026 The Radius Authors.
# Licensed under the Apache License, Version 2.0.
# You may obtain a copy of the License at
# http://www.apache.org/licenses/LICENSE-2.0

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
work_dir="$(mktemp -d "${TMPDIR:-/tmp}/radius-local-functional-test.XXXXXX")"
trap 'rm -rf "${work_dir}"' EXIT

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

assert_contains() {
    [[ "$1" == *"$2"* ]] || fail "expected '$1' to contain '$2'"
}

assert_not_contains() {
    [[ "$1" != *"$2"* ]] || fail "did not expect '$1' to contain '$2'"
}

export REAL_MAKE
REAL_MAKE="$(command -v make)"
mkdir -p "${work_dir}/build/scripts" "${work_dir}/bin" \
    "${work_dir}/dist/local-functional" \
    "${work_dir}/test/functional-portable/dynamicrp/noncloud/resources/testdata" \
    "${work_dir}/hack/bicep-types-radius/generated" \
    "${work_dir}/.github/actions/install-git-http-backend"
cp "${SCRIPT_DIR}/local-functional.sh" "${work_dir}/build/scripts/"
extension_fixture="test/functional-portable/dynamicrp/noncloud/resources/testdata/testResourceSchema-validAny.bicep"
cp "${SCRIPT_DIR}/../../${extension_fixture}" "${work_dir}/${extension_fixture}"
runner="${work_dir}/build/scripts/local-functional.sh"
state="${work_dir}/dist/local-functional"
printf '{}\n' >"${work_dir}/hack/bicep-types-radius/generated/index.json"
cat >"${work_dir}/.github/actions/install-git-http-backend/install-git-http-backend.sh" <<'EOF'
if [[ "${MOCK_FIXTURE_CONCURRENCY}" == true ]]; then
    bash "${MOCK_FIXTURE_HELPER}" git-http
fi
EOF
export MOCK_LOG="${work_dir}/commands.log"
export GOPATH="${work_dir}/gopath" GOCACHE="${work_dir}/gocache"
export LOCAL_FUNCTIONAL_HOST_OS=linux LOCAL_FUNCTIONAL_HOST_ARCH=amd64
export LOCAL_FUNCTIONAL_SUITE=""
export MOCK_FAILURE="" MOCK_BUILD_STATUS=77 MOCK_TEST_STATUS=0
export MOCK_PULL_STATUS=0 MOCK_SIGNAL="" MOCK_MAKEFILE=""
export MOCK_FIXTURE_CONCURRENCY="" MOCK_FIXTURE_TEST_STATUS=0 MOCK_FIXTURE_SIGNAL=""
export MOCK_FIXTURE_HELPER="${work_dir}/fixture-worker.sh"
export MOCK_CLUSTERS="" MOCK_CONTAINERS="" MOCK_NETWORKS=""
export MOCK_DOCKER_PLATFORM=linux/x86_64
export MOCK_GOTEST_OPTS="-count=1"
unset MAKEFLAGS MFLAGS GOTEST_OPTS
cat >"${MOCK_FIXTURE_HELPER}" <<'EOF'
#!/bin/bash
set -euo pipefail
touch "${MOCK_LOG}.fixture-$1"
for attempt in {1..50}; do
    [[ -f "${MOCK_LOG}.fixtures-release" ]] && break
    sleep 0.02
done
[[ -f "${MOCK_LOG}.fixtures-release" ]] || exit 1
sleep 0.2
touch "${MOCK_LOG}.fixture-$1-done"
EOF

# Only filesystem utilities and GNU Make are real; infrastructure is mocked.
for tool in dirname mkdir mktemp mv rm grep bash cp touch rmdir sleep sed cat chmod ln; do
    ln -s "$(command -v "${tool}")" "${work_dir}/bin/${tool}"
done
cat >"${work_dir}/bin/mock" <<'EOF'
#!/bin/bash
set -euo pipefail
tool="${0##*/}"
printf '%s %s\n' "${tool}" "$*" >>"${MOCK_LOG}"
if [[ "${tool} $*" == "${MOCK_FAILURE}" ]]; then
    echo "mock failure: ${tool} $*" >&2
    exit 1
fi
if [[ "${MOCK_FIXTURE_CONCURRENCY}" == true && "${tool}" =~ ^(flux|dapr)$ ]]; then
    bash "${MOCK_FIXTURE_HELPER}" "${tool}"
fi
case "${tool}" in
git)
    case "${1:-}" in
    clone) mkdir -p "${7}/.git" ;;
    -C)
        if [[ "${3:-}" == pull ]]; then
            exit "${MOCK_PULL_STATUS}"
        fi
        echo deadbeef1
        ;;
    *) echo "Unexpected git command: $*" >&2; exit 1 ;;
    esac
    ;;
go | helm | python3 | flux | envsubst | update-ca-certificates | dapr | rad) ;;
yq)
    case "${1:-}" in
    -e) echo /planes/radius/local/resourceGroups/kind-radius ;;
    -i)
        printf 'environment=%s\n' "${ENVIRONMENT_ID}" >>"${MOCK_LOG}"
        ;;
    *) echo "Unexpected yq command: $*" >&2; exit 1 ;;
    esac
    ;;
bicep)
    case "${1:-}" in
    restore)
        grep -F '"radius": "br:localhost:5443/radius:deadbeef1"' \
            test/bicepconfig.json >/dev/null
        grep -F '"testresources": "br:localhost:5443/testresources:deadbeef1"' \
            test/functional-portable/dynamicrp/noncloud/resources/bicepconfig.json >/dev/null
        mkdir -p "${HOME}/.bicep/br"
        touch "${HOME}/.bicep/br/extensions-ready"
        ;;
    build)
        [[ -f "${HOME}/.bicep/br/extensions-ready" ]] || {
            echo "BCP190: local extension not restored" >&2
            exit 1
        }
        ;;
    *) ;;
    esac
    ;;
openssl)
    if [[ "${2:-}" == -help ]]; then
        echo '-addext'
    else
        while [[ "$#" -gt 0 ]]; do
            case "$1" in
            -keyout | -out) touch "$2"; shift 2 ;;
            *) shift ;;
            esac
        done
    fi
    ;;
docker)
    case "${1:-} ${2:-}" in
    "info --format") printf '%s\n' "${MOCK_DOCKER_PLATFORM}" ;;
    "container inspect") exit 1 ;;
    "container ls") printf '%s\n' "${MOCK_CONTAINERS}" ;;
    "network ls") printf '%s\n' "${MOCK_NETWORKS}" ;;
    "inspect radius-local-registry") echo 127.0.0.1 ;;
    *) ;;
    esac
    ;;
k3d)
    case "${1:-} ${2:-}" in
    "cluster list") echo '[]' ;;
    "kubeconfig get") echo 'mock kubeconfig' ;;
    *) ;;
    esac
    ;;
jq)
    cat >/dev/null
    case "$1" in
    -e) exit 1 ;;
    -r) printf '%s\n' "${MOCK_CLUSTERS}" ;;
    *) exit 1 ;;
    esac
    ;;
kubectl)
    if [[ "${1:-}" == port-forward ]]; then
        printf '%s\n' "$$" >"${MOCK_LOG}.forward-pid"
        exec sleep 60
    fi
    ;;
curl)
    if [[ -n "${MOCK_MAKEFILE}" ]]; then
        for attempt in {1..50}; do
            [[ -f "${MOCK_LOG}.forward-pid" ]] && exit 0
            sleep 0.02
        done
        echo "mock port-forward did not start" >&2
        exit 1
    fi
    ;;
sudo)
    if [[ "${1:-}" == tee ]]; then
        cat >>"${MOCK_LOG}"
    fi
    ;;
make)
    if [[ -n "${MOCK_MAKEFILE}" && "${1:-}" == test-functional-* ]]; then
        exec "${REAL_MAKE}" --no-print-directory -f "${MOCK_MAKEFILE}" "$@"
    fi
    case "${1:-}" in
    build-rad) exit "${MOCK_BUILD_STATUS}" ;;
    install-bicep)
        directory="${2#BICEP_INSTALL_DIR=}"
        mkdir -p "${directory}"
        cp "$0" "${directory}/bicep"
        chmod +x "${directory}/bicep"
        ;;
    publish-test-terraform-recipes)
        if [[ "${MOCK_FIXTURE_CONCURRENCY}" == true ]]; then
            for attempt in {1..50}; do
                [[ -f "${MOCK_LOG}.fixture-flux" &&
                    -f "${MOCK_LOG}.fixture-dapr" &&
                    -f "${MOCK_LOG}.fixture-git-http" ]] && break
                sleep 0.02
            done
            [[ -f "${MOCK_LOG}.fixture-flux" &&
                -f "${MOCK_LOG}.fixture-dapr" &&
                -f "${MOCK_LOG}.fixture-git-http" ]] || exit 1
            touch "${MOCK_LOG}.fixtures-release"
            if [[ -n "${MOCK_FIXTURE_SIGNAL}" ]]; then
                kill "-${MOCK_FIXTURE_SIGNAL}" "${PPID}"
            fi
            exit "${MOCK_FIXTURE_TEST_STATUS}"
        fi
        ;;
    publish-test-bicep-recipes)
        [[ "${BICEP_RECIPE_PUBLISH_PARALLELISM}" == 4 ]] || exit 1
        [[ -d dist/local-functional/test.lock ]] || {
            echo "recipes published without the fixture lock" >&2
            exit 1
        }
        grep -F '"radius": "br:localhost:5443/radius:deadbeef1"' \
            test/bicepconfig.json >/dev/null
        grep -F '"testresources": "br:localhost:5443/testresources:deadbeef1"' \
            test/functional-portable/dynamicrp/noncloud/resources/bicepconfig.json >/dev/null
        ;;
    test-functional-*)
        [[ -f "${HOME}/.bicep/br/extensions-ready" ]] || {
            echo "tests started without restoring extensions" >&2
            exit 1
        }
        printf 'external=%s\n' "${RADIUS_TEST_EXTERNAL_KUBECONFIG-<unset>}" \
            >>"${MOCK_LOG}"
        cat test/bicepconfig.json >>"${MOCK_LOG}"
        if [[ -n "${MOCK_SIGNAL}" ]]; then
            kill "-${MOCK_SIGNAL}" "${PPID}"
        fi
        exit "${MOCK_TEST_STATUS}"
        ;;
    *) ;;
    esac
    ;;
*) echo "Unexpected mock command: ${tool} $*" >&2; exit 1 ;;
esac
EOF
chmod +x "${work_dir}/bin/mock"
for tool in git go docker make k3d kubectl helm openssl jq yq python3 \
    flux curl envsubst sudo update-ca-certificates bicep dapr rad; do
    ln -s mock "${work_dir}/bin/${tool}"
done
export PATH="${work_dir}/bin"

run_runner() {
    local expected_status="$1" status=0
    shift
    : >"${MOCK_LOG}"
    rm -f "${MOCK_LOG}.forward-pid"
    rm -f "${MOCK_LOG}.fixtures-release"
    for fixture in flux dapr git-http; do
        rm -f "${MOCK_LOG}.fixture-${fixture}" "${MOCK_LOG}.fixture-${fixture}-done"
    done
    rm -f "${state}/home${LOCAL_FUNCTIONAL_SUITE:+-${LOCAL_FUNCTIONAL_SUITE}}/.bicep/br/extensions-ready"
    output="$(bash "${runner}" "$@" 2>&1)" || status=$?
    [[ "${status}" == "${expected_status}" ]] ||
        fail "expected exit ${expected_status}, got ${status}: ${output}"
    calls="$(<"${MOCK_LOG}")"
}

for platform in linux/arm windows/amd64 windows/arm64; do
    export LOCAL_FUNCTIONAL_HOST_OS="${platform%/*}"
    export LOCAL_FUNCTIONAL_HOST_ARCH="${platform#*/}"
    run_runner 1 setup
    assert_contains "${output}" "requires Debian/Ubuntu or macOS"
    assert_not_contains "${calls}" "docker "
    assert_not_contains "${calls}" "make "
done
export LOCAL_FUNCTIONAL_HOST_OS=linux LOCAL_FUNCTIONAL_HOST_ARCH=amd64
for tool in envsubst update-ca-certificates yq; do
    rm "${work_dir}/bin/${tool}"
    run_runner 1 setup
    assert_contains "${output}" "${tool}"
    assert_not_contains "${calls}" "docker "
    assert_not_contains "${calls}" "make "
    ln -s mock "${work_dir}/bin/${tool}"
done

for platform in windows/amd64 linux/arm; do
    export MOCK_DOCKER_PLATFORM="${platform}"
    run_runner 1 setup
    assert_contains "${output}" "Unsupported Docker daemon platform: ${platform}"
    assert_not_contains "${calls}" "make "
    assert_not_contains "${calls}" "sudo "
done
export MOCK_DOCKER_PLATFORM=linux/aarch64
export LOCAL_FUNCTIONAL_HOST_ARCH=arm64
export MOCK_FAILURE="docker run --rm --platform linux/amd64 alpine:3.24.2 /bin/true"
run_runner 1 setup
assert_contains "${output}" "must support linux/amd64 emulation"
assert_not_contains "${calls}" "make "
assert_not_contains "${calls}" "sudo "
assert_not_contains "${calls}" "docker network create"
export MOCK_FAILURE=""
for platform in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do
    export LOCAL_FUNCTIONAL_HOST_OS="${platform%/*}"
    export LOCAL_FUNCTIONAL_HOST_ARCH="${platform#*/}"
    for daemon in linux/x86_64 linux/amd64 linux/aarch64 linux/arm64; do
        export MOCK_DOCKER_PLATFORM="${daemon}"
        run_runner 77 setup
        case "${daemon}" in
        linux/x86_64 | linux/amd64)
            arch=amd64
            assert_not_contains "${calls}" "docker run --rm --platform"
            ;;
        *)
            arch=arm64
            assert_contains "${calls}" \
                "docker run --rm --platform linux/amd64 alpine:3.24.2 /bin/true"
            ;;
        esac
        assert_contains "${calls}" \
            "make build-rad docker-build DOCKER_REGISTRY=radius-local DOCKER_TAG_VERSION=deadbeef1 DOCKER_BUILD_ARCH=${arch}"
        assert_not_contains "${calls}" "sudo "
    done
done
export LOCAL_FUNCTIONAL_HOST_OS=linux LOCAL_FUNCTIONAL_HOST_ARCH=amd64
export MOCK_DOCKER_PLATFORM=linux/x86_64

for existing in both results container_logs neither; do
    for directory in results container_logs; do
        mkdir -p "${state}/${directory}"
        if [[ "${existing}" == both || "${existing}" == "${directory}" ]]; then
            printf 'prior %s\n' "${directory}" >"${state}/${directory}/sentinel"
        else
            rmdir "${state}/${directory}"
        fi
    done
    run_runner 77 setup
    assert_not_contains "${calls}" "sudo "
    for directory in results container_logs; do
        [[ -d "${state}/${directory}" && ! -e "${state}/${directory}/sentinel" ]] ||
            fail "new ${directory} was not empty"
        if [[ "${existing}" == both || "${existing}" == "${directory}" ]]; then
            archive="${output#*Previous test diagnostics retained in }"
            archive="${archive%%$'\n'*}"
            archive="${archive%.}"
            [[ "$(<"${archive}/${directory}/sentinel")" == "prior ${directory}" ]] ||
                fail "old ${directory} was not preserved"
        fi
    done
    if [[ "${existing}" == neither ]]; then
        assert_not_contains "${output}" "Previous test diagnostics retained"
    fi
done

export MOCK_BUILD_STATUS=0
printf 'original config\n' >"${work_dir}/test/bicepconfig.json"
dynamic_config="${work_dir}/test/functional-portable/dynamicrp/noncloud/resources/bicepconfig.json"
for platform in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do
    export LOCAL_FUNCTIONAL_HOST_OS="${platform%/*}"
    export LOCAL_FUNCTIONAL_HOST_ARCH="${platform#*/}"
    export MOCK_DOCKER_PLATFORM="linux/${LOCAL_FUNCTIONAL_HOST_ARCH}"
    export MOCK_FAILURE=""
    expected=0
    if [[ "${LOCAL_FUNCTIONAL_HOST_OS}" == linux ]]; then
        # Stop before the real host's inotify limit is inspected.
        export MOCK_FAILURE="docker network create radius-functional"
        expected=1
    fi
    run_runner "${expected}" setup
    if [[ "${LOCAL_FUNCTIONAL_HOST_OS}" == linux ]]; then
        assert_contains "${output}" "mock failure: ${MOCK_FAILURE}"
        assert_contains "${calls}" \
            "sudo install -m 644 ${state}/certs/registry.crt /usr/local/share/ca-certificates/radius-local-functional.crt"
        assert_contains "${calls}" "sudo update-ca-certificates"
    else
        assert_contains "${calls}" \
            "sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain ${state}/certs/registry.crt"
        assert_not_contains "${calls}" "update-ca-certificates"
    fi
    assert_contains "${calls}" \
        "127.0.0.1 radius-local-registry # radius-local-functional"
    assert_contains "${calls}" "DOCKER_BUILD_ARCH=${LOCAL_FUNCTIONAL_HOST_ARCH}"
    [[ "$(<"${work_dir}/test/bicepconfig.json")" == "original config" ]] ||
        fail "${platform} setup did not restore existing config"
    [[ ! -e "${dynamic_config}" && ! -e "${state}/test.lock" ]] ||
        fail "${platform} setup leaked fixtures or a lock"
done
export MOCK_FAILURE=""
export LOCAL_FUNCTIONAL_HOST_OS=linux LOCAL_FUNCTIONAL_HOST_ARCH=amd64
export MOCK_DOCKER_PLATFORM=linux/x86_64
for suite in "" database multicluster upgrade dynamicrp; do
    export LOCAL_FUNCTIONAL_SUITE="${suite}"
    run_runner 0 install
    assert_contains "${calls}" \
        "rad deploy ./test/infra/radius/legacy-test-environment.bicep --parameters name=kind-radius --parameters namespace=default"
    assert_contains "${calls}" \
        "yq -e .workspaces.items[.workspaces.default].scope ${state}/home${suite:+-${suite}}/.rad/config.yaml"
    assert_contains "${calls}" \
        "yq -i .workspaces.items[.workspaces.default].environment = strenv(ENVIRONMENT_ID) ${state}/home${suite:+-${suite}}/.rad/config.yaml"
    assert_contains "${calls}" \
        "environment=/planes/radius/local/resourceGroups/kind-radius/providers/Applications.Core/environments/kind-radius"
    assert_not_contains "${calls}" "rad env "
done
export LOCAL_FUNCTIONAL_SUITE=""
for command in \
    "rad deploy ./test/infra/radius/legacy-test-environment.bicep --parameters name=kind-radius --parameters namespace=default" \
    "yq -e .workspaces.items[.workspaces.default].scope ${state}/home/.rad/config.yaml" \
    "yq -i .workspaces.items[.workspaces.default].environment = strenv(ENVIRONMENT_ID) ${state}/home/.rad/config.yaml"; do
    export MOCK_FAILURE="${command}"
    run_runner 1 install
    assert_contains "${output}" "mock failure: ${command}"
    assert_not_contains "${calls}" "environment="
done
export MOCK_FAILURE=""
[[ -d "${state}/samples/.git" ]] || fail "samples were not cloned"
run_runner 0 fixtures
assert_contains "${calls}" "git -C ${state}/samples pull --ff-only origin edge"
export MOCK_PULL_STATUS=77
run_runner 77 fixtures
export MOCK_PULL_STATUS=0
export MOCK_FIXTURE_CONCURRENCY=true
for outcome in success failure signal; do
    export MOCK_FIXTURE_TEST_STATUS=0 MOCK_FIXTURE_SIGNAL=""
    expected=0
    if [[ "${outcome}" == failure ]]; then
        export MOCK_FIXTURE_TEST_STATUS=77
        expected=77
    elif [[ "${outcome}" == signal ]]; then
        export MOCK_FIXTURE_SIGNAL=TERM
        expected=143
    fi
    run_runner "${expected}" fixtures
    for fixture in flux dapr git-http; do
        [[ -f "${MOCK_LOG}.fixture-${fixture}-done" ]] ||
            fail "${outcome} did not drain ${fixture}"
    done
    [[ "$(<"${work_dir}/test/bicepconfig.json")" == "original config" ]] ||
        fail "${outcome} fixture publication did not restore existing config"
    [[ ! -e "${dynamic_config}" && ! -e "${state}/test.lock" ]] ||
        fail "${outcome} fixture publication leaked fixtures or a lock"
done
export MOCK_FIXTURE_CONCURRENCY="" MOCK_FIXTURE_TEST_STATUS=0 MOCK_FIXTURE_SIGNAL=""
export MOCK_FAILURE="make publish-test-bicep-recipes BICEP_RECIPE_REGISTRY=localhost:5443"
run_runner 1 fixtures
assert_contains "${output}" "mock failure: ${MOCK_FAILURE}"
[[ "$(<"${work_dir}/test/bicepconfig.json")" == "original config" ]] ||
    fail "recipe publication failure did not restore existing config"
[[ ! -e "${dynamic_config}" && ! -e "${state}/test.lock" ]] ||
    fail "recipe publication failure leaked fixtures or a lock"
export MOCK_FAILURE=""
mkdir "${state}/test.lock"
printf '%s\n' "$$" >"${state}/test.lock/owner"
run_runner 1 fixtures
assert_contains "${output}" "Another local functional test run"
assert_not_contains "${calls}" "make publish-"
assert_not_contains "${calls}" "flux install"
printf '2147483647\n' >"${state}/test.lock/owner"
run_runner 0 fixtures
[[ ! -e "${state}/test.lock" ]] || fail "fixture publication did not recover stale lock"
export MOCK_FAILURE="flux install --namespace=flux-system --version=v2.6.4 --components=source-controller --network-policy=false"
run_runner 1 fixtures
assert_contains "${output}" "mock failure"
export MOCK_FAILURE=""

for suite in "" database statestore upgrade multicluster dynamicrp; do
    export LOCAL_FUNCTIONAL_SUITE="${suite}"
    export RADIUS_TEST_EXTERNAL_KUBECONFIG=/unrelated/kubeconfig
    # shellcheck disable=SC2016 # Expanded after the child selects its environment.
    run_runner 0 exec bash -c 'printf "%s" "${RADIUS_TEST_EXTERNAL_KUBECONFIG-<unset>}"'
    if [[ "${suite}" == multicluster ]]; then
        [[ "${output}" == "${state}/external-host.kubeconfig" ]] ||
            fail "multicluster selected an unrelated kubeconfig"
    else
        [[ "${output}" == "<unset>" ]] || fail "${suite} inherited an external cluster"
    fi
done
unset RADIUS_TEST_EXTERNAL_KUBECONFIG
export LOCAL_FUNCTIONAL_SUITE=multicluster
run_runner 1 test test-functional-multicluster-noncloud
assert_contains "${output}" "Missing external cluster kubeconfig"
assert_not_contains "${calls}" "kubectl "
touch "${state}/external-host.kubeconfig"
for command in \
    "bicep restore ${extension_fixture} --force" \
    "bicep build ${extension_fixture} --stdout --no-restore"; do
    export MOCK_FAILURE="${command}"
    run_runner 1 test test-functional-multicluster-noncloud
    assert_contains "${output}" "mock failure: ${command}"
    assert_not_contains "${calls}" "make "
    assert_not_contains "${calls}" "kubectl port-forward"
    [[ "$(<"${work_dir}/test/bicepconfig.json")" == "original config" ]] ||
        fail "extension bootstrap failure did not restore config"
    [[ ! -e "${dynamic_config}" && ! -e "${state}/test.lock" ]] ||
        fail "extension bootstrap failure leaked fixtures or a lock"
done
export MOCK_FAILURE=""
for outcome in success failure signal; do
    export MOCK_TEST_STATUS=0 MOCK_SIGNAL=""
    expected=0
    if [[ "${outcome}" == failure ]]; then
        export MOCK_TEST_STATUS=77
        expected=77
    elif [[ "${outcome}" == signal ]]; then
        export MOCK_SIGNAL=TERM
        expected=143
    fi
    run_runner "${expected}" test test-functional-multicluster-noncloud
    assert_contains "${calls}" "bicep restore ${extension_fixture} --force"
    assert_contains "${calls}" \
        "bicep build ${extension_fixture} --stdout --no-restore"
    assert_contains "${calls}" '"radius": "br:localhost:5443/radius:deadbeef1"'
    [[ "$(<"${work_dir}/test/bicepconfig.json")" == "original config" ]] ||
        fail "${outcome} did not restore existing config"
    [[ ! -e "${dynamic_config}" ]] || fail "${outcome} did not restore absent config"
    [[ ! -e "${state}/test.lock" ]] || fail "${outcome} leaked the fixture lock"
done
export MOCK_TEST_STATUS=0 MOCK_SIGNAL=""
mkdir "${state}/test.lock"
printf '%s\n' "$$" >"${state}/test.lock/owner"
run_runner 1 test test-functional-multicluster-noncloud
assert_contains "${output}" "Another local functional test run"
assert_not_contains "${calls}" "make "
printf '2147483647\n' >"${state}/test.lock/owner"
run_runner 0 test test-functional-multicluster-noncloud
[[ ! -e "${state}/test.lock" ]] || fail "stale lock was not recovered"

export LOCAL_FUNCTIONAL_SUITE=""
export MOCK_MAKEFILE="${work_dir}/serial.mk"
export SERIAL_STATE="${work_dir}/serial-state"
mkdir "${SERIAL_STATE}"
cat >"${MOCK_MAKEFILE}" <<'EOF'
.PHONY: test-functional-all-noncloud group-one group-two
test-functional-all-noncloud: group-one group-two
group-one group-two:
	@mkdir "$(SERIAL_STATE)/active" 2>/dev/null || { echo "parallel groups detected" >&2; exit 1; }; \
	trap 'rmdir "$(SERIAL_STATE)/active"' EXIT; \
	echo "$@" >>"$(SERIAL_STATE)/completed"
EOF
for parallelism in inherited forwarded both; do
    export MAKEFLAGS=""
    args=(test test-functional-all-noncloud)
    if [[ "${parallelism}" != forwarded ]]; then
        export MAKEFLAGS=-j8
    fi
    if [[ "${parallelism}" != inherited ]]; then
        args+=(-j12)
    fi
    run_runner 0 "${args[@]}"
    [[ "$(<"${SERIAL_STATE}/completed")" == $'group-one\ngroup-two' ]] ||
        fail "${parallelism} did not run groups serially"
    rm "${SERIAL_STATE}/completed"
    forward_pid="$(<"${MOCK_LOG}.forward-pid")"
    ! kill -0 "${forward_pid}" 2>/dev/null || fail "port-forward survived the test run"
done
# Only the negative control waits for a peer, so serial runs cannot deadlock.
export MOCK_MAKEFILE="${work_dir}/unserialized.mk"
cat >"${MOCK_MAKEFILE}" <<'EOF'
.PHONY: test-functional-all-noncloud group-one group-two
test-functional-all-noncloud: group-one group-two
group-one group-two:
	@bash "$(SERIAL_STATE)/overlap-worker.sh" "$@"
EOF
cat >"${SERIAL_STATE}/overlap-worker.sh" <<'EOF'
#!/bin/bash
set -euo pipefail
if [[ "$1" == group-one ]]; then
    peer=group-two
else
    peer=group-one
    sleep 0.3
fi
touch "${SERIAL_STATE}/$1-ready"
for attempt in {1..200}; do
    if [[ -f "${SERIAL_STATE}/${peer}-ready" ]]; then
        echo "parallel groups detected" >&2
        exit 1
    fi
    sleep 0.05
done
echo "Timed out waiting for ${peer} to start" >&2
exit 1
EOF
sed 's/make "\$@" --jobs=1/make "\$@"/' "${runner}" \
    >"${work_dir}/build/scripts/unserialized.sh"
runner="${work_dir}/build/scripts/unserialized.sh"
run_runner 2 test test-functional-all-noncloud -j12
assert_contains "${output}" "parallel groups detected"
assert_not_contains "${output}" "Timed out waiting"
forward_pid="$(<"${MOCK_LOG}.forward-pid")"
! kill -0 "${forward_pid}" 2>/dev/null || fail "failed run leaked its port-forward"
[[ ! -e "${state}/test.lock" ]] || fail "failed run leaked its fixture lock"
[[ "$(<"${work_dir}/test/bicepconfig.json")" == "original config" ]] ||
    fail "failed run did not restore its fixture config"
runner="${work_dir}/build/scripts/local-functional.sh"
unset MAKEFLAGS
export MOCK_MAKEFILE=""

export MOCK_MAKEFILE="${work_dir}/concurrent.mk"
export MOCK_FAILED_TARGET=""
export LOCAL_FUNCTIONAL_HOST_OS=darwin
rm -f "${SERIAL_STATE}/completed"
mkdir -p "${state}/home/.rad" "${SERIAL_STATE}/active"
printf 'standard CLI config\n' >"${state}/home/.rad/config.yaml"
cat >"${MOCK_MAKEFILE}" <<'EOF'
test-functional-%:
	@bash "$(SERIAL_STATE)/worker.sh" "$@"
EOF
cat >"${SERIAL_STATE}/worker.sh" <<'EOF'
#!/bin/bash
set -euo pipefail
target="$1"
[[ -d "dist/local-functional/test.lock" ]] || exit 1
[[ -f "${HOME}/.bicep/br/extensions-ready" ]] || exit 1
printf '%s\n' "${target}" >"${HOME}/.bicep/br/worker-owner"
[[ "${RADIUS_TEST_FAST_CLEANUP}" == false ]] || exit 1
if [[ "${target}" == test-functional-corerp-noncloud ]]; then
    [[ -f "${SERIAL_STATE}/ucp-ready" ]] || exit 1
fi
if [[ -z "${LOCAL_FUNCTIONAL_SUITE}" ]]; then
    [[ "${HOME}" == *"/home-${target}" ]] || exit 1
    [[ "$(<"${HOME}/.rad/config.yaml")" == "standard CLI config" ]] || exit 1
    if [[ "${target}" == test-functional-corerp-noncloud ]]; then
        [[ "${GOTEST_OPTS}" == "${MOCK_GOTEST_OPTS} -parallel 10" ]] || exit 1
    else
        [[ "${GOTEST_OPTS}" == "${MOCK_GOTEST_OPTS} -parallel 5" ]] || exit 1
    fi
    mkdir "${SERIAL_STATE}/active/${target}"
    trap 'rmdir "${SERIAL_STATE}/active/${target}"' EXIT
    slots=("${SERIAL_STATE}"/active/*)
    ((${#slots[@]} <= 3)) || exit 1
    if ((${#slots[@]} > 1)); then
        touch "${SERIAL_STATE}/overlap"
    fi
else
    [[ "${HOME}" == *"/home-${LOCAL_FUNCTIONAL_SUITE}" ]] || exit 1
    if [[ "${LOCAL_FUNCTIONAL_SUITE}" == dynamicrp ]]; then
        [[ "${GOTEST_OPTS}" == "${MOCK_GOTEST_OPTS} -parallel 5" ]] || exit 1
    else
        [[ "${GOTEST_OPTS}" == "${MOCK_GOTEST_OPTS}" ]] || exit 1
    fi
fi
printf '%s\n' "${target}" >>"${SERIAL_STATE}/started"
if [[ -n "${MOCK_SIGNAL}" && "${target}" == test-functional-corerp-noncloud ]]; then
    kill "-${MOCK_SIGNAL}" "$(<dist/local-functional/test.lock/owner)"
fi
sleep 0.2
[[ "${target}" != "${MOCK_FAILED_TARGET}" ]] || {
    echo "intentional worker failure: ${target}" >&2
    exit 77
}
printf '%s\n' "${target}" >>"${SERIAL_STATE}/completed"
if [[ "${target}" == test-functional-ucp-noncloud ]]; then
    touch "${SERIAL_STATE}/ucp-ready"
fi
EOF
export MAKEFLAGS=-j12
run_runner 0 test --all-groups
[[ -f "${SERIAL_STATE}/overlap" ]] || fail "standard groups never overlapped"
completed=()
while IFS= read -r target; do completed+=("${target}"); done \
    <"${SERIAL_STATE}/completed"
[[ "${#completed[@]}" == 13 ]] || fail "full lifecycle omitted test groups"
[[ "$(<"${state}/home/.rad/config.yaml")" == "standard CLI config" ]] ||
    fail "concurrent workers changed the standard CLI config"
[[ ! -e "${state}/home/.bicep/br/worker-owner" ]] ||
    fail "concurrent workers shared the standard extension cache"
for target in "${completed[@]}"; do
    suite="${target#test-functional-}"
    suite="${suite%-noncloud}"
    if [[ "${suite}" =~ ^(database|multicluster|statestore|upgrade|dynamicrp)$ ]]; then
        cache="${state}/home-${suite}/.bicep/br"
    else
        cache="${state}/home-${target}/.bicep/br"
    fi
    [[ "$(<"${cache}/worker-owner")" == "${target}" ]] ||
        fail "${target} did not have an independent extension cache"
done
[[ ! -e "${state}/test.lock" && ! -e "${dynamic_config}" ]] ||
    fail "concurrent run leaked fixtures or a lock"
[[ "$(<"${work_dir}/test/bicepconfig.json")" == "original config" ]] ||
    fail "concurrent run did not restore fixtures"
for suite in database multicluster statestore upgrade dynamicrp; do
    assert_contains "${calls}" "k3d cluster delete radius-functional-${suite}"
done
assert_contains "${calls}" "k3d cluster delete radius-functional-external"
rm "${SERIAL_STATE}/started" "${SERIAL_STATE}/completed"
export GOTEST_OPTS="-race -coverprofile ./dist/unit_test/ut_coverage.out"
export MOCK_GOTEST_OPTS="${GOTEST_OPTS}"
run_runner 0 test --all-groups
completed=()
while IFS= read -r target; do completed+=("${target}"); done \
    <"${SERIAL_STATE}/completed"
[[ "${#completed[@]}" == 13 ]] ||
    fail "custom test options run omitted test groups"
rm "${SERIAL_STATE}/started" "${SERIAL_STATE}/completed"
unset GOTEST_OPTS
export MOCK_GOTEST_OPTS="-count=1"
export MOCK_FAILED_TARGET=test-functional-database-noncloud
run_runner 1 test --all-groups
assert_contains "${output}" "intentional worker failure"
assert_contains "${output}" "Local functional workers failed"
assert_not_contains "${calls}" "k3d cluster delete radius-functional-database"
started=() completed=()
while IFS= read -r target; do started+=("${target}"); done \
    <"${SERIAL_STATE}/started"
while IFS= read -r target; do completed+=("${target}"); done \
    <"${SERIAL_STATE}/completed"
[[ "${#started[@]}" == 13 && "${#completed[@]}" == 12 ]] ||
    fail "failed run did not drain the other workers"
[[ ! -e "${state}/test.lock" && ! -e "${dynamic_config}" ]] ||
    fail "failed concurrent run leaked fixtures or a lock"
forward_pid="$(<"${MOCK_LOG}.forward-pid")"
! kill -0 "${forward_pid}" 2>/dev/null ||
    fail "failed concurrent run leaked its port-forward"
rm "${SERIAL_STATE}/started" "${SERIAL_STATE}/completed" \
    "${SERIAL_STATE}/ucp-ready"
export MOCK_FAILED_TARGET="" MOCK_SIGNAL=TERM
run_runner 143 test --all-groups
completed=()
while IFS= read -r target; do completed+=("${target}"); done \
    <"${SERIAL_STATE}/completed"
[[ "${#completed[@]}" == 13 ]] ||
    fail "interrupted run restored fixtures before draining workers"
[[ ! -e "${state}/test.lock" && ! -e "${dynamic_config}" ]] ||
    fail "interrupted concurrent run leaked fixtures or a lock"
forward_pid="$(<"${MOCK_LOG}.forward-pid")"
! kill -0 "${forward_pid}" 2>/dev/null ||
    fail "interrupted concurrent run leaked its port-forward"
unset MAKEFLAGS
export MOCK_MAKEFILE="" MOCK_FAILED_TARGET="" MOCK_SIGNAL=""
export LOCAL_FUNCTIONAL_HOST_OS=linux

export MOCK_CLUSTERS=$'radius-functional\nradius-functional-multicluster\nradius-functional-unrelated'
export MOCK_CONTAINERS=$'radius-local-registry\nradius-local-registry-unrelated'
export MOCK_NETWORKS=$'radius-functional\nradius-functional-unrelated'
run_runner 0 cleanup
assert_contains "${output}" "Infrastructure removed;"
assert_not_contains "${calls}" "delete radius-functional-unrelated"
assert_not_contains "${calls}" "rm -fv radius-local-registry-unrelated"
assert_not_contains "${calls}" "rm radius-functional-unrelated"
[[ -d "${state}/results" && -d "${state}/container_logs" ]] ||
    fail "cleanup removed retained diagnostics"
for command in \
    "k3d cluster list -o json" \
    "k3d cluster delete radius-functional" \
    "docker container ls --all --format {{.Names}}" \
    "docker container rm -fv radius-local-registry" \
    "docker network ls --format {{.Name}}" \
    "docker network rm radius-functional" \
    "sudo rm -f /usr/local/share/ca-certificates/radius-local-functional.crt" \
    "sudo update-ca-certificates" \
    "sudo sed -i.radius-local-functional /^127\.0\.0\.1 radius-local-registry # radius-local-functional$/d /etc/hosts" \
    "sudo rm -f /etc/hosts.radius-local-functional"; do
    export MOCK_FAILURE="${command}"
    run_runner 1 cleanup
    assert_contains "${output}" "Cleanup incomplete;"
    assert_not_contains "${output}" "Infrastructure removed;"
    assert_contains "${calls}" "sudo sed -i"
done
export MOCK_FAILURE="" MOCK_CLUSTERS="" MOCK_CONTAINERS="" MOCK_NETWORKS=""
run_runner 0 cleanup
assert_not_contains "${calls}" "k3d cluster delete"
assert_not_contains "${calls}" "docker container rm"
assert_not_contains "${calls}" "docker network rm"
export LOCAL_FUNCTIONAL_HOST_OS=darwin LOCAL_FUNCTIONAL_HOST_ARCH=arm64
run_runner 0 cleanup
assert_contains "${calls}" \
    "sudo security delete-certificate -c radius-local-registry /Library/Keychains/System.keychain"
assert_not_contains "${calls}" "update-ca-certificates"
export MOCK_FAILURE="sudo security delete-certificate -c radius-local-registry /Library/Keychains/System.keychain"
run_runner 1 cleanup
assert_contains "${output}" "Cleanup incomplete;"

echo "local-functional runner tests passed"
