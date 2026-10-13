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
printf 'exit 0\n' \
    >"${work_dir}/.github/actions/install-git-http-backend/install-git-http-backend.sh"
export MOCK_LOG="${work_dir}/commands.log"
export GOPATH="${work_dir}/gopath" GOCACHE="${work_dir}/gocache"
export LOCAL_FUNCTIONAL_HOST_OS=linux LOCAL_FUNCTIONAL_HOST_ARCH=amd64
export LOCAL_FUNCTIONAL_SUITE=""
export MOCK_FAILURE="" MOCK_BUILD_STATUS=77 MOCK_TEST_STATUS=0
export MOCK_PULL_STATUS=0 MOCK_SIGNAL="" MOCK_MAKEFILE=""
export MOCK_CLUSTERS="" MOCK_CONTAINERS="" MOCK_NETWORKS=""
unset MAKEFLAGS MFLAGS

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
bicep)
    case "${1:-}" in
    restore)
        grep -F '"radius": "br:localhost:5443/radius:deadbeef1"' \
            test/bicepconfig.json >/dev/null
        grep -F '"testresources": "br:localhost:5443/testresources:deadbeef1"' \
            test/functional-portable/dynamicrp/noncloud/resources/bicepconfig.json >/dev/null
        mkdir -p "${HOME}"
        touch "${HOME}/.bicep-extensions-ready"
        ;;
    build)
        [[ -f "${HOME}/.bicep-extensions-ready" ]] || {
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
    if [[ -n "${MOCK_MAKEFILE}" ]]; then
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
    test-functional-*)
        [[ -f "${HOME}/.bicep-extensions-ready" ]] || {
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
for tool in git go docker make k3d kubectl helm openssl jq python3 \
    flux curl envsubst sudo update-ca-certificates bicep dapr rad; do
    ln -s mock "${work_dir}/bin/${tool}"
done
export PATH="${work_dir}/bin"

run_runner() {
    local expected_status="$1" status=0
    shift
    : >"${MOCK_LOG}"
    rm -f "${MOCK_LOG}.forward-pid"
    rm -f "${state}/home${LOCAL_FUNCTIONAL_SUITE:+-${LOCAL_FUNCTIONAL_SUITE}}/.bicep-extensions-ready"
    output="$(bash "${runner}" "$@" 2>&1)" || status=$?
    [[ "${status}" == "${expected_status}" ]] ||
        fail "expected exit ${expected_status}, got ${status}: ${output}"
    calls="$(<"${MOCK_LOG}")"
}

for platform in linux/arm64 windows/amd64; do
    export LOCAL_FUNCTIONAL_HOST_OS="${platform%/*}"
    export LOCAL_FUNCTIONAL_HOST_ARCH="${platform#*/}"
    run_runner 1 setup
    assert_contains "${output}" "requires Debian/Ubuntu on amd64"
    assert_not_contains "${calls}" "docker "
    assert_not_contains "${calls}" "make "
done
export LOCAL_FUNCTIONAL_HOST_OS=linux LOCAL_FUNCTIONAL_HOST_ARCH=amd64
for tool in envsubst update-ca-certificates; do
    rm "${work_dir}/bin/${tool}"
    run_runner 1 setup
    assert_contains "${output}" "${tool}"
    assert_not_contains "${calls}" "docker "
    assert_not_contains "${calls}" "make "
    ln -s mock "${work_dir}/bin/${tool}"
done

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
for platform in linux/amd64 darwin/amd64 darwin/arm64; do
    export LOCAL_FUNCTIONAL_HOST_OS="${platform%/*}"
    export LOCAL_FUNCTIONAL_HOST_ARCH="${platform#*/}"
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
done
export MOCK_FAILURE=""
export LOCAL_FUNCTIONAL_HOST_OS=linux LOCAL_FUNCTIONAL_HOST_ARCH=amd64
[[ -d "${state}/samples/.git" ]] || fail "samples were not cloned"
run_runner 0 fixtures
assert_contains "${calls}" "git -C ${state}/samples pull --ff-only origin edge"
export MOCK_PULL_STATUS=77
run_runner 77 fixtures
export MOCK_PULL_STATUS=0

for suite in "" database statestore upgrade multicluster; do
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
run_runner 1 test test-functional-multicluster-noncloud
assert_contains "${output}" "Missing external cluster kubeconfig"
assert_not_contains "${calls}" "kubectl "
touch "${state}/external-host.kubeconfig"
printf 'original config\n' >"${work_dir}/test/bicepconfig.json"
dynamic_config="${work_dir}/test/functional-portable/dynamicrp/noncloud/resources/bicepconfig.json"
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
	sleep 0.2; \
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
# The same fixture must detect concurrency without the serialization fix.
sed 's/make "\$@" --jobs=1/make "\$@"/' "${runner}" \
    >"${work_dir}/build/scripts/unserialized.sh"
runner="${work_dir}/build/scripts/unserialized.sh"
run_runner 2 test test-functional-all-noncloud -j12
assert_contains "${output}" "parallel groups detected"
forward_pid="$(<"${MOCK_LOG}.forward-pid")"
! kill -0 "${forward_pid}" 2>/dev/null || fail "failed run leaked its port-forward"
[[ ! -e "${state}/test.lock" ]] || fail "failed run leaked its fixture lock"
[[ "$(<"${work_dir}/test/bicepconfig.json")" == "original config" ]] ||
    fail "failed run did not restore its fixture config"
runner="${work_dir}/build/scripts/local-functional.sh"
unset MAKEFLAGS
export MOCK_MAKEFILE=""

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
