#!/bin/bash

# Copyright 2026 The Radius Authors.
# Licensed under the Apache License, Version 2.0.
# You may obtain a copy of the License at
# http://www.apache.org/licenses/LICENSE-2.0

# Run non-cloud functional tests against an isolated, locally built installation.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
readonly REPO_ROOT
readonly STATE="${REPO_ROOT}/dist/local-functional"
readonly CLUSTER="radius-functional"
readonly REGISTRY="radius-local-registry"
readonly REGISTRY_PORT="5443"
readonly HOST_OS="${LOCAL_FUNCTIONAL_HOST_OS:-$(go env GOOS)}"
readonly HOST_ARCH="${LOCAL_FUNCTIONAL_HOST_ARCH:-$(go env GOARCH)}"
TAG="$(git -C "${REPO_ROOT}" rev-parse --short=9 HEAD)"
readonly TAG
readonly CA_FILE="${STATE}/certs/registry.crt"
readonly LINUX_SYSTEM_CA="/usr/local/share/ca-certificates/radius-local-functional.crt"
readonly MACOS_SYSTEM_KEYCHAIN="/Library/Keychains/System.keychain"
readonly HOSTS_ENTRY="127.0.0.1 ${REGISTRY} # radius-local-functional"
readonly TEST_LOCK="${STATE}/test.lock"
readonly IMAGES=(ucpd applications-rp dynamic-rp controller bicep pre-upgrade
    testrp magpiego)
worker_pids=()
export LOCAL_FUNCTIONAL_SUITE="${LOCAL_FUNCTIONAL_SUITE:-}"
case "${LOCAL_FUNCTIONAL_SUITE}" in
"" | database | multicluster | statestore | upgrade | dynamicrp) ;;
*)
    echo "Invalid LOCAL_FUNCTIONAL_SUITE: ${LOCAL_FUNCTIONAL_SUITE}" >&2
    exit 2
    ;;
esac

cd "${REPO_ROOT}"

configure_environment() {
    local suffix="${LOCAL_FUNCTIONAL_SUITE:+-${LOCAL_FUNCTIONAL_SUITE}}"
    export GOPATH="${GOPATH:-$(go env GOPATH)}"
    export GOCACHE="${GOCACHE:-$(go env GOCACHE)}"
    export HOME="${STATE}/home${suffix}"
    export KUBECONFIG="${STATE}/kubeconfig${suffix}"
    if [[ "${LOCAL_FUNCTIONAL_SUITE}" == "multicluster" ]]; then
        export RADIUS_TEST_EXTERNAL_KUBECONFIG="${STATE}/external-host.kubeconfig"
    else
        unset RADIUS_TEST_EXTERNAL_KUBECONFIG
    fi
    export PATH="${STATE}/bin:${REPO_ROOT}/dist/${HOST_OS}_${HOST_ARCH}/release:${PATH}"
    export RAD_PATH="${REPO_ROOT}/dist/${HOST_OS}_${HOST_ARCH}/release"
    export SSL_CERT_FILE="${CA_FILE}"
    export BICEP_TRUSTED_REGISTRIES="localhost,${REGISTRY}"
    export DOCKER_REGISTRY="${REGISTRY}:${REGISTRY_PORT}"
    export REL_VERSION="${TAG}"
    export BICEP_RECIPE_REGISTRY="${DOCKER_REGISTRY}"
    export BICEP_RECIPE_TAG_VERSION="${TAG}"
    export TF_RECIPE_MODULE_SERVER_URL="http://tf-module-server.radius-test-tf-module-server.svc.cluster.local"
    export RADIUS_REGISTRY_CERT_FILE="${CA_FILE}"
    export RADIUS_CHART_LOCATION="${REPO_ROOT}/deploy/Chart"
    export RADIUS_CONTAINER_LOG_PATH="${STATE}/container_logs"
    export RADIUS_SAMPLES_REPO_ROOT="${STATE}/samples"
    export GIT_HTTP_SERVER_URL="http://localhost:30080"
    export GIT_HTTP_USERNAME="testuser"
    export GIT_HTTP_EMAIL="testuser@radapp.io"
    export GIT_HTTP_PASSWORD="not-a-secret-password"
    export DE_IMAGE="ghcr.io/radius-project/deployment-engine"
    export DE_TAG="latest"
    export RADIUS_TEST_FAST_CLEANUP=false
    export TEST_TIMEOUT="${TEST_TIMEOUT:-1h}"
}

create_cluster() {
    local name="$1"
    local kubeconfig="$2"
    local api_port
    if [[ "${HOST_OS}" == "linux" ]]; then
        local watcher_limit
        watcher_limit="$(</proc/sys/fs/inotify/max_user_instances)"
        if ((watcher_limit < 1024)); then
            echo "Local clusters need fs.inotify.max_user_instances >= 1024." >&2
            echo "Run: sudo sysctl -w fs.inotify.max_user_instances=1024" >&2
            exit 1
        fi
    fi
    case "${name}" in
    "${CLUSTER}") api_port=6550 ;;
    "${CLUSTER}-database") api_port=6551 ;;
    "${CLUSTER}-multicluster") api_port=6552 ;;
    "${CLUSTER}-statestore") api_port=6553 ;;
    "${CLUSTER}-upgrade") api_port=6554 ;;
    "${CLUSTER}-external") api_port=6555 ;;
    "${CLUSTER}-dynamicrp") api_port=6556 ;;
    *)
        echo "Unknown local test cluster: ${name}" >&2
        exit 2
        ;;
    esac
    local registry_ip
    registry_ip="$(docker inspect "${REGISTRY}" \
        --format "{{(index .NetworkSettings.Networks \"${CLUSTER}\").IPAddress}}")"
    k3d cluster create "${name}" --network "${CLUSTER}" \
        --kubeconfig-update-default=false --kubeconfig-switch-context=false \
        --api-port "127.0.0.1:${api_port}" \
        --k3s-arg "--disable=traefik@server:0" \
        --host-alias "${registry_ip}:${REGISTRY}" \
        --volume "${CA_FILE}:/etc/ssl/certs/radius-local-functional.crt@all" \
        --registry-config "${STATE}/registries.yaml" --wait --timeout 180s
    k3d kubeconfig get "${name}" >"${kubeconfig}"
    for attempt in {1..36}; do
        if kubectl --kubeconfig "${kubeconfig}" \
            get node "k3d-${name}-server-0" >/dev/null; then
            break
        fi
        echo "Waiting for ${name} node registration (${attempt}/36)..."
        sleep 5
    done
    kubectl --kubeconfig "${kubeconfig}" wait --for=condition=Ready \
        node/"k3d-${name}-server-0" --timeout=180s
}

install_radius() {
    rad install kubernetes --chart "${RADIUS_CHART_LOCATION}" \
        --set "global.imageRegistry=${DOCKER_REGISTRY}" \
        --set "global.imageTag=${TAG}" \
        --set "de.image=${DE_IMAGE},de.tag=${DE_TAG}" \
        --set dashboard.image=ghcr.io/radius-project/dashboard \
        --set dashboard.tag=latest \
        --set-file "global.rootCA.cert=${CA_FILE}" "$@"
    kubectl get gatewayclass contour
    kubectl wait --for=condition=Programmed gateway/radius \
        -n radius-system --timeout=180s
    rad workspace create kubernetes
    rad group create kind-radius
    rad group switch kind-radius
    rad env create kind-radius --namespace default
    rad env switch kind-radius
}

install_host_ca() {
    case "${HOST_OS}" in
    linux)
        sudo install -m 644 "${CA_FILE}" "${LINUX_SYSTEM_CA}"
        sudo update-ca-certificates
        ;;
    darwin)
        sudo security add-trusted-cert -d -r trustRoot \
            -k "${MACOS_SYSTEM_KEYCHAIN}" "${CA_FILE}"
        ;;
    esac
}

remove_host_ca() {
    local status=0
    case "${HOST_OS}" in
    linux)
        sudo rm -f "${LINUX_SYSTEM_CA}" || status=1
        sudo update-ca-certificates || status=1
        ;;
    darwin)
        if sudo security find-certificate -c "${REGISTRY}" \
            "${MACOS_SYSTEM_KEYCHAIN}" >/dev/null 2>&1; then
            sudo security delete-certificate -c "${REGISTRY}" \
                "${MACOS_SYSTEM_KEYCHAIN}" || status=1
        fi
        ;;
    esac
    return "${status}"
}

setup() {
    case "${HOST_OS}/${HOST_ARCH}" in
    linux/amd64 | darwin/amd64 | darwin/arm64) ;;
    *)
        echo "This local runner requires Debian/Ubuntu on amd64 or macOS on amd64/arm64." >&2
        exit 1
        ;;
    esac
    for tool in docker git go make k3d kubectl helm openssl jq python3 \
        flux curl envsubst sudo; do
        command -v "${tool}" || {
            echo "Missing required tool: ${tool}. See running-functional-tests.md." >&2
            exit 1
        }
    done
    if [[ "${HOST_OS}" == "linux" ]]; then
        command -v update-ca-certificates || {
            echo "Linux setup requires Debian/Ubuntu certificate tooling (update-ca-certificates)." >&2
            exit 1
        }
    fi
    openssl req -help 2>&1 | grep -q -- "-addext" || {
        echo "The installed OpenSSL must support 'openssl req -addext'." >&2
        exit 1
    }
    docker info >/dev/null
    [[ -f hack/bicep-types-radius/generated/index.json ]] || {
        echo "Generate local Bicep types with make generate-bicep-types first." >&2
        exit 1
    }
    if docker container inspect "${REGISTRY}" >/dev/null 2>&1; then
        echo "Registry ${REGISTRY} already exists; use cleanup first." >&2
        exit 1
    fi
    if k3d cluster list -o json | jq -e \
        --arg name "${CLUSTER}" 'any(.[]; .name == $name)' >/dev/null; then
        echo "Cluster ${CLUSTER} already exists; use cleanup first." >&2
        exit 1
    fi
    if [[ -d "${STATE}/results" || -d "${STATE}/container_logs" ]]; then
        mkdir -p "${STATE}/results-history"
        local previous_run directory
        previous_run="$(mktemp -d "${STATE}/results-history/run.XXXXXX")"
        for directory in results container_logs; do
            if [[ -d "${STATE}/${directory}" ]]; then
                mv "${STATE}/${directory}" "${previous_run}/${directory}"
            fi
        done
        echo "Previous test diagnostics retained in ${previous_run}."
    fi
    mkdir -p "${STATE}"/{certs,home,bin,results,container_logs}
    rm -f "${STATE}/home/.rad/config.yaml"
    make build-rad docker-build \
        DOCKER_REGISTRY=radius-local DOCKER_TAG_VERSION="${TAG}"
    make install-bicep BICEP_INSTALL_DIR="${STATE}/bin"
    make install-bicep BICEP_INSTALL_DIR="${STATE}/home/.rad/bin"
    make install-dapr DAPR_INSTALL_DIR="${STATE}/bin"

    openssl req -x509 -nodes -days 30 -newkey rsa:2048 \
        -subj "/CN=${REGISTRY}" \
        -addext "subjectAltName=DNS:localhost,DNS:${REGISTRY}" \
        -addext "basicConstraints=critical,CA:TRUE" \
        -keyout "${STATE}/certs/registry.key" -out "${CA_FILE}"
    chmod 600 "${STATE}/certs/registry.key"
    install_host_ca
    printf '%s\n' "${HOSTS_ENTRY}" | sudo tee -a /etc/hosts
    docker network create "${CLUSTER}"
    docker run -d --name "${REGISTRY}" --network "${CLUSTER}" \
        -p "127.0.0.1:${REGISTRY_PORT}:${REGISTRY_PORT}" \
        -v "${STATE}/certs:/certs:ro" \
        -e "REGISTRY_HTTP_ADDR=0.0.0.0:${REGISTRY_PORT}" \
        -e REGISTRY_HTTP_TLS_CERTIFICATE=/certs/registry.crt \
        -e REGISTRY_HTTP_TLS_KEY=/certs/registry.key registry:2
    curl --fail --retry 10 --retry-all-errors --retry-delay 1 \
        --cacert "${CA_FILE}" "https://localhost:${REGISTRY_PORT}/v2/"
    cat >"${STATE}/registries.yaml" <<EOF
mirrors:
  "${REGISTRY}:${REGISTRY_PORT}":
    endpoint:
      - "https://${REGISTRY}:${REGISTRY_PORT}"
configs:
  "${REGISTRY}:${REGISTRY_PORT}":
    tls:
      ca_file: /etc/ssl/certs/radius-local-functional.crt
EOF
    create_cluster "${CLUSTER}" "${STATE}/kubeconfig"
    mkdir -p "${STATE}/home/.kube"
    cp "${STATE}/kubeconfig" "${STATE}/home/.kube/config"
    for image in "${IMAGES[@]}"; do
        docker tag "radius-local/${image}:${TAG}" \
            "localhost:${REGISTRY_PORT}/${image}:${TAG}"
        docker push "localhost:${REGISTRY_PORT}/${image}:${TAG}"
    done

    configure_environment
    kubectl cluster-info
    rad version
    install_radius
    fixtures
    echo "Local test infrastructure is ready."
}

fixtures() (
    configure_environment
    local fixture_pids=()
    trap 'trap "" INT TERM; wait_workers "${fixture_pids[@]}" || true' EXIT
    trap 'exit 130' INT
    trap 'exit 143' TERM
    (
        flux install --namespace=flux-system --version=v2.6.4 \
            --components=source-controller --network-policy=false
        kubectl wait --for=condition=available deployment/source-controller \
            -n flux-system --timeout=180s
    ) &
    fixture_pids+=("$!")
    (
        bash .github/actions/install-git-http-backend/install-git-http-backend.sh \
            "${GIT_HTTP_USERNAME}" "${GIT_HTTP_PASSWORD}" \
            git-http-backend alpine/git:2.45.2
    ) &
    fixture_pids+=("$!")
    (
        dapr init -k --wait --timeout 600 --runtime-version 1.18.2
    ) &
    fixture_pids+=("$!")
    make publish-test-terraform-recipes
    bicep publish-extension hack/bicep-types-radius/generated/index.json \
        --target "br:localhost:${REGISTRY_PORT}/radius:${TAG}" --force
    rad bicep publish-extension \
        -f test/functional-portable/dynamicrp/noncloud/resources/testdata/testresourcetypes.yaml \
        --target "br:localhost:${REGISTRY_PORT}/testresources:${TAG}" --force
    BICEP_RECIPE_PUBLISH_PARALLELISM=4 make publish-test-bicep-recipes \
        BICEP_RECIPE_REGISTRY="localhost:${REGISTRY_PORT}"
    if [[ ! -d "${STATE}/samples/.git" ]]; then
        git clone --depth 1 --branch edge \
            https://github.com/radius-project/samples.git "${STATE}/samples"
    else
        git -C "${STATE}/samples" pull --ff-only origin edge
    fi
    wait_workers "${fixture_pids[@]}"
    trap - EXIT INT TERM
)

restore_configs() {
    local config
    for config in test/bicepconfig.json \
        test/functional-portable/dynamicrp/noncloud/resources/bicepconfig.json; do
        if [[ -f "${config_backup}/${config}" ]]; then
            cp "${config_backup}/${config}" "${config}"
        elif [[ -f "${config_backup}/${config}.absent" ]]; then
            rm -f "${config}"
        fi
    done
}

finish_tests() {
    # Workers share the temporary fixtures, so drain them before restoration.
    trap '' INT TERM
    wait_workers "${worker_pids[@]}" || true
    worker_pids=()
    if [[ -n "${git_forward_pid:-}" ]]; then
        if kill -0 "${git_forward_pid}" 2>/dev/null; then
            kill "${git_forward_pid}"
            wait "${git_forward_pid}" || true
        fi
    fi
    unset git_forward_pid
    restore_configs
    if [[ "${test_lock_owned:-false}" == "true" ]]; then
        rm -f "${TEST_LOCK}/owner"
        rmdir "${TEST_LOCK}"
        test_lock_owned=false
    fi
}

restore_extensions() {
    local extension_fixture="test/functional-portable/dynamicrp/noncloud/resources/testdata/testResourceSchema-validAny.bicep"
    echo "Restoring local Bicep extensions before parallel test compilation..."
    bicep restore "${extension_fixture}" --force
    bicep build "${extension_fixture}" --stdout --no-restore >/dev/null
}

copy_extensions() {
    mkdir -p "${HOME}/.bicep/br"
    cp -R "${STATE}/home/.bicep/br/." "${HOME}/.bicep/br"
}

run_standard_group() {
    local target="$1"
    local parallel=5
    if [[ "${target}" == "test-functional-corerp-noncloud" ]]; then
        parallel=10
    fi
    export HOME="${STATE}/home-${target}"
    export RADIUS_CONTAINER_LOG_PATH="${STATE}/container_logs/${target}"
    mkdir -p "${HOME}/.rad/bin" "${HOME}/.kube"
    cp "${STATE}/home/.rad/config.yaml" "${HOME}/.rad/config.yaml"
    cp "${STATE}/kubeconfig" "${HOME}/.kube/config"
    cp "${STATE}/bin/bicep" "${HOME}/.rad/bin/bicep"
    copy_extensions
    GOTEST_OPTS="${GOTEST_OPTS} -parallel ${parallel}" \
        make "${target}" --jobs=1 GOTESTSUM_JSONFILE_DIR="${STATE}/results"
}

wait_workers() {
    local pid status=0
    for pid in "$@"; do
        if ! wait "${pid}"; then
            status=1
        fi
    done
    return "${status}"
}

run_all_groups() {
    local suite target lane status=0 ucp_pid
    local lane_targets=() standard_pids=() isolated_pids=()
    local targets=(
        test-functional-corerp-noncloud
        test-functional-cli-noncloud
        test-functional-kubernetes-noncloud
        test-functional-daprrp-noncloud
        test-functional-datastoresrp-noncloud
        test-functional-msgrp-noncloud
        test-functional-samples-noncloud
        test-functional-ucp-noncloud
    )
    # Make remains serial; only this full lifecycle owns concurrent workers.
    for suite in database multicluster statestore upgrade dynamicrp; do
        (
            prepare_isolated "${suite}"
            export RADIUS_CONTAINER_LOG_PATH="${STATE}/container_logs/${suite}"
            isolated_opts="${GOTEST_OPTS}"
            if [[ "${suite}" == "dynamicrp" ]]; then
                # Custom routes must not affect standard provider-discovery tests.
                rad resource-provider create --from-file \
                    test/functional-portable/dynamicrp/noncloud/resources/testdata/testresourcetypes.yaml
                isolated_opts+=" -parallel 5"
            fi
            copy_extensions
            GOTEST_OPTS="${isolated_opts}" \
                make "test-functional-${suite}-noncloud" --jobs=1 \
                GOTESTSUM_JSONFILE_DIR="${STATE}/results"
            k3d cluster delete "${CLUSTER}-${suite}"
            if [[ "${suite}" == "multicluster" ]]; then
                k3d cluster delete "${CLUSTER}-external"
            fi
        ) >"${STATE}/results/${suite}.log" 2>&1 &
        isolated_pids+=("$!")
        worker_pids+=("$!")
    done
    # UCP registers the provider that Core RP's discovery test must also list.
    (
        run_standard_group test-functional-ucp-noncloud
    ) >"${STATE}/results/test-functional-ucp-noncloud.log" 2>&1 &
    ucp_pid="$!"
    worker_pids+=("${ucp_pid}")
    wait_workers "${ucp_pid}" || status=1
    if ((status == 0)); then
        for lane in \
            "test-functional-corerp-noncloud" \
            "test-functional-cli-noncloud test-functional-daprrp-noncloud" \
            "test-functional-kubernetes-noncloud test-functional-datastoresrp-noncloud test-functional-msgrp-noncloud test-functional-samples-noncloud"; do
            read -r -a lane_targets <<<"${lane}"
            (
                for target in "${lane_targets[@]}"; do
                    run_standard_group "${target}" \
                        >"${STATE}/results/${target}.log" 2>&1
                done
            ) &
            standard_pids+=("$!")
            worker_pids+=("$!")
        done
    fi
    wait_workers "${standard_pids[@]}" || status=1
    wait_workers "${isolated_pids[@]}" || status=1
    for suite in database multicluster statestore upgrade dynamicrp; do
        cat "${STATE}/results/${suite}.log"
    done
    for target in "${targets[@]}"; do
        if [[ -f "${STATE}/results/${target}.log" ]]; then
            cat "${STATE}/results/${target}.log"
        fi
    done
    if ((status != 0)); then
        echo "Local functional workers failed; see ${STATE}/results." >&2
    fi
    return "${status}"
}

acquire_test_lock() {
    local owner
    if mkdir "${TEST_LOCK}" 2>/dev/null; then
        printf '%s\n' "$$" >"${TEST_LOCK}/owner"
        test_lock_owned=true
        return
    fi
    if [[ -f "${TEST_LOCK}/owner" ]]; then
        owner="$(<"${TEST_LOCK}/owner")"
        if [[ "${owner}" =~ ^[0-9]+$ ]] && ! kill -0 "${owner}" 2>/dev/null; then
            rm -f "${TEST_LOCK}/owner"
            rmdir "${TEST_LOCK}"
            mkdir "${TEST_LOCK}"
            printf '%s\n' "$$" >"${TEST_LOCK}/owner"
            test_lock_owned=true
            return
        fi
    fi
    echo "Another local functional test run owns the Bicep fixtures." >&2
    exit 1
}

run_tests() {
    configure_environment
    if [[ "${LOCAL_FUNCTIONAL_SUITE}" == "multicluster" &&
        ! -f "${RADIUS_TEST_EXTERNAL_KUBECONFIG}" ]]; then
        echo "Missing external cluster kubeconfig: ${RADIUS_TEST_EXTERNAL_KUBECONFIG}" >&2
        echo "Provision the multicluster suite before running its tests." >&2
        exit 1
    fi
    kubectl cluster-info
    test_lock_owned=false
    acquire_test_lock
    config_backup="$(mktemp -d "${STATE}/backup.XXXXXX")"
    trap finish_tests EXIT
    trap 'exit 130' INT
    trap 'exit 143' TERM
    local config
    for config in test/bicepconfig.json \
        test/functional-portable/dynamicrp/noncloud/resources/bicepconfig.json; do
        mkdir -p "${config_backup}/$(dirname "${config}")"
        if [[ -f "${config}" ]]; then
            cp "${config}" "${config_backup}/${config}"
        else
            touch "${config_backup}/${config}.absent"
        fi
        cat >"${config}" <<EOF
{
  "extensions": {
    "radius": "br:localhost:${REGISTRY_PORT}/radius:${TAG}",
    "aws": "br:biceptypes.azurecr.io/aws:latest",
    "testresources": "br:localhost:${REGISTRY_PORT}/testresources:${TAG}"
  }
}
EOF
    done
    restore_extensions
    if [[ -z "${LOCAL_FUNCTIONAL_SUITE:-}" ]]; then
        kubectl port-forward -n git-http-backend svc/git-http 30080:3000 \
            >"${STATE}/results/git-port-forward.log" 2>&1 &
        git_forward_pid="$!"
        curl --silent --show-error --retry 15 --retry-connrefused --retry-delay 1 \
            "${GIT_HTTP_SERVER_URL}" >/dev/null
        kill -0 "${git_forward_pid}"
    fi
    export GOTEST_OPTS="${GOTEST_OPTS:--count=1}"
    if [[ "${1:-}" == "--all-groups" ]]; then
        run_all_groups
    else
        make "$@" --jobs=1 GOTESTSUM_JSONFILE_DIR="${STATE}/results"
    fi
    finish_tests
    trap - EXIT INT TERM
}

prepare_isolated() {
    case "${1:-}" in
    database | multicluster | statestore | upgrade | dynamicrp) ;;
    *)
        echo "Expected database, multicluster, statestore, upgrade, or dynamicrp." >&2
        exit 2
        ;;
    esac
    export LOCAL_FUNCTIONAL_SUITE="$1"
    configure_environment
    local name="${CLUSTER}-${LOCAL_FUNCTIONAL_SUITE}"
    if k3d cluster list -o json | jq -e \
        --arg name "${name}" 'any(.[]; .name == $name)' >/dev/null; then
        echo "${name} already exists. Use LOCAL_FUNCTIONAL_SUITE=$1 with test to retry, or delete that cluster before provisioning again." >&2
        exit 1
    fi
    create_cluster "${name}" "${KUBECONFIG}"
    mkdir -p "${HOME}/.kube" "${HOME}/.rad/bin"
    cp "${KUBECONFIG}" "${HOME}/.kube/config"
    cp "${STATE}/bin/bicep" "${HOME}/.rad/bin/bicep"
    rm -f "${HOME}/.rad/config.yaml"

    local install_args=()
    if [[ "${LOCAL_FUNCTIONAL_SUITE}" == "multicluster" ]]; then
        create_cluster "${CLUSTER}-external" "${STATE}/external-host.kubeconfig"
        cp "${STATE}/external-host.kubeconfig" "${STATE}/external-pod.kubeconfig"
        local external_ip
        external_ip="$(docker inspect "k3d-${CLUSTER}-external-server-0" \
            --format "{{(index .NetworkSettings.Networks \"${CLUSTER}\").IPAddress}}")"
        kubectl --kubeconfig "${STATE}/external-pod.kubeconfig" \
            config unset "clusters.k3d-${CLUSTER}-external.certificate-authority-data"
        kubectl --kubeconfig "${STATE}/external-pod.kubeconfig" \
            config set-cluster "k3d-${CLUSTER}-external" \
            --server="https://${external_ip}:6443" --insecure-skip-tls-verify=true
        kubectl create namespace radius-system
        kubectl create secret generic target-kubeconfig -n radius-system \
            --from-file="kubeconfig=${STATE}/external-pod.kubeconfig"
        install_args+=(--set global.targetCluster.enabled=true)
    elif [[ "${LOCAL_FUNCTIONAL_SUITE}" == "database" ]]; then
        install_args+=(--set database.enabled=true)
    fi
    if [[ "${LOCAL_FUNCTIONAL_SUITE}" != "statestore" ]]; then
        install_radius "${install_args[@]}"
    fi
    make publish-test-terraform-recipes
}

isolated() {
    prepare_isolated "$@"
    run_tests "test-functional-${LOCAL_FUNCTIONAL_SUITE}-noncloud"
    k3d cluster delete "${CLUSTER}-${LOCAL_FUNCTIONAL_SUITE}"
    if [[ "${LOCAL_FUNCTIONAL_SUITE}" == "multicluster" ]]; then
        k3d cluster delete "${CLUSTER}-external"
    fi
}

cleanup() {
    local status=0 names name
    if names="$(k3d cluster list -o json | jq -r '.[].name')"; then
        for name in "${CLUSTER}" "${CLUSTER}-database" \
            "${CLUSTER}-multicluster" "${CLUSTER}-statestore" \
            "${CLUSTER}-upgrade" "${CLUSTER}-external" \
            "${CLUSTER}-dynamicrp"; do
            if grep -Fxq -- "${name}" <<<"${names}"; then
                k3d cluster delete "${name}" || status=1
            fi
        done
    else
        status=1
    fi
    if names="$(docker container ls --all --format '{{.Names}}')"; then
        if grep -Fxq -- "${REGISTRY}" <<<"${names}"; then
            docker container rm -fv "${REGISTRY}" || status=1
        fi
    else
        status=1
    fi
    if names="$(docker network ls --format '{{.Name}}')"; then
        if grep -Fxq -- "${CLUSTER}" <<<"${names}"; then
            docker network rm "${CLUSTER}" || status=1
        fi
    else
        status=1
    fi
    remove_host_ca || status=1
    sudo sed -i.radius-local-functional \
        '/^127\.0\.0\.1 radius-local-registry # radius-local-functional$/d' \
        /etc/hosts || status=1
    sudo rm -f /etc/hosts.radius-local-functional || status=1
    if ((status != 0)); then
        echo "Cleanup incomplete; resolve the errors above and rerun cleanup." >&2
        return "${status}"
    fi
    echo "Infrastructure removed; logs and isolated CLI configuration remain in ${STATE}."
}

case "${1:-}" in
all)
    export LOCAL_FUNCTIONAL_SUITE=""
    setup
    run_tests --all-groups
    ;;
setup) setup ;;
test)
    shift
    if [[ "$#" == 0 ]]; then
        set -- test-functional-all-noncloud
    fi
    run_tests "$@"
    ;;
exec)
    shift
    configure_environment
    "$@"
    ;;
install)
    shift
    configure_environment
    install_radius "$@"
    ;;
fixtures) fixtures ;;
isolated)
    shift
    isolated "$@"
    ;;
prepare-isolated)
    shift
    prepare_isolated "$@"
    ;;
cleanup) cleanup ;;
*)
    echo "Usage: bash $0 {all|setup|fixtures|test [make arguments...]|isolated suite|prepare-isolated suite|exec command...|install [flags...]|cleanup}"
    exit 2
    ;;
esac
