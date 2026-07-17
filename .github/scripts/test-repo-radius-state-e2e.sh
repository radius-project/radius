#!/bin/bash

# Proves Repo Radius state can round-trip through GHCR while the workload
# remains on a separate Kubernetes cluster.

set -euo pipefail

readonly APP_NAME="repo-radius-state-e2e"
readonly RESOURCE_NAME="repo-radius-state-container"
readonly ENVIRONMENT_NAME="repo-radius-state-e2e"
readonly WORKLOAD_NAMESPACE="repo-radius-state-e2e"
readonly WORKSPACE_NAME="repo-radius-state-e2e"
readonly SELECTOR="radapp.io/application=${APP_NAME},radapp.io/resource=${RESOURCE_NAME}"
readonly REPOSITORY_ROOT="${GITHUB_WORKSPACE:-$(git rev-parse --show-toplevel)}"
readonly DIAGNOSTICS_DIR="${REPOSITORY_ROOT}/dist/repo-radius-state-e2e"
readonly SOURCE_APP_FILE="${REPOSITORY_ROOT}/test/functional-portable/statestore/noncloud/testdata/repo-radius-state-app.bicep"
readonly RUN_SUFFIX="${GITHUB_RUN_ID:-local}-${GITHUB_RUN_ATTEMPT:-1}"
readonly NETWORK_NAME="repo-radius-state-${RUN_SUFFIX}"
readonly REGISTRY_CONTAINER="repo-radius-registry-${RUN_SUFFIX}"
readonly REGISTRY_ALIAS="repo-radius-registry"
readonly CLUSTER_REGISTRY="${REGISTRY_ALIAS}:5000"
readonly WORKLOAD_CLUSTER="radius-workload-${RUN_SUFFIX}"
readonly CONTROL_PLANE_A="radius-cp-a-${RUN_SUFFIX}"
readonly CONTROL_PLANE_B="radius-cp-b-${RUN_SUFFIX}"
readonly WORK_DIR="${RUNNER_TEMP:-/tmp}/repo-radius-state-${RUN_SUFFIX}"
readonly HOST_WORKLOAD_KUBECONFIG="${WORK_DIR}/workload-host.kubeconfig"
readonly INTERNAL_WORKLOAD_KUBECONFIG="${WORK_DIR}/workload-internal.kubeconfig"
readonly REGISTRY_CONFIG="${WORK_DIR}/registries.yaml"
readonly APP_FILE="${WORK_DIR}/repo-radius-state-app.bicep"
readonly STATE_OWNED_MARKER="${WORK_DIR}/state-owned"
readonly SAVED_DIGEST_FILE="${WORK_DIR}/saved-state-digest"
readonly PHASE_DIR="${WORK_DIR}/phases"
readonly BOOTSTRAP_TAG="bootstrap"

: "${LOCAL_DOCKER_REGISTRY:?LOCAL_DOCKER_REGISTRY must be set}"
: "${DOCKER_TAG_VERSION:?DOCKER_TAG_VERSION must be set}"
: "${GH_TOKEN:?GH_TOKEN must be set}"
: "${RADIUS_STATE_ARCHIVE:?RADIUS_STATE_ARCHIVE must be set}"
: "${RADIUS_STATE_BACKEND:?RADIUS_STATE_BACKEND must be set}"
: "${RADIUS_STATE_REGISTRY:?RADIUS_STATE_REGISTRY must be set}"

export RADIUS_PREVIEW=true
export RADIUS_STATE_REGISTRY="${RADIUS_STATE_REGISTRY,,}"
readonly STATE_REFERENCE="${RADIUS_STATE_REGISTRY}:${RADIUS_STATE_ARCHIVE}"

PACKAGE_API=""
PACKAGE_NAME=""

usage() {
    cat <<EOF
Usage: $(basename "$0") <phase>

Phases:
  validate-state-package
  prepare-workload
  install-initial-control-plane
  deploy-initial
  persist-state
  replace-control-plane
  restore-state
  update-workload
  diagnostics
  cleanup
  all
EOF
}

mark_phase() {
    mkdir -p "${PHASE_DIR}"
    touch "${PHASE_DIR}/$1"
}

begin_phase() {
    mkdir -p "${WORK_DIR}" "${PHASE_DIR}"
    printf '%s\n' "$1" >"${WORK_DIR}/current-phase"
    append_summary ""
    append_summary "### $2"
}

require_phase() {
    local phase="$1"
    if [[ ! -f "${PHASE_DIR}/${phase}" ]]; then
        echo "Required phase '${phase}' has not completed." >&2
        return 1
    fi
}

append_summary() {
    if [[ -n "${GITHUB_STEP_SUMMARY:-}" ]]; then
        printf '%s\n' "$*" >>"${GITHUB_STEP_SUMMARY}"
    fi
}

urlencode() {
    jq -nr --arg value "$1" '$value | @uri'
}

configure_package_api() {
    local registry_path owner owner_type owner_scope
    registry_path="${RADIUS_STATE_REGISTRY#ghcr.io/}"
    owner="${registry_path%%/*}"
    PACKAGE_NAME="${registry_path#*/}"
    if [[ "${registry_path}" == "${RADIUS_STATE_REGISTRY}" ||
        -z "${owner}" || -z "${PACKAGE_NAME}" ]]; then
        echo "RADIUS_STATE_REGISTRY must match ghcr.io/<owner>/<package>." >&2
        return 1
    fi
    if [[ ! "${PACKAGE_NAME}" =~ ^[A-Za-z0-9][A-Za-z0-9._/-]*$ ]]; then
        echo "Invalid GHCR package name: ${PACKAGE_NAME}" >&2
        return 1
    fi

    owner_type="$(gh api "/users/${owner}" --jq '.type')"
    case "${owner_type}" in
        Organization) owner_scope="orgs" ;;
        User) owner_scope="users" ;;
        *)
            echo "Unsupported GitHub owner type: ${owner_type}" >&2
            return 1
            ;;
    esac
    PACKAGE_API="/${owner_scope}/$(urlencode "${owner}")/packages/container/$(urlencode "${PACKAGE_NAME}")"
}

fetch_package_metadata() {
    configure_package_api
    mkdir -p "${WORK_DIR}"
    gh api "${PACKAGE_API}" >"${WORK_DIR}/package.json"
}

verify_package_metadata() {
    local visibility linked_repository
    visibility="$(jq -r '.visibility // empty' "${WORK_DIR}/package.json")"
    case "${visibility}" in
        private | internal) ;;
        *)
            echo "State package must be private or internal; found ${visibility:-missing}." >&2
            return 1
            ;;
    esac

    linked_repository="$(jq -r '.repository.full_name // empty' \
        "${WORK_DIR}/package.json")"
    if [[ -n "${GITHUB_REPOSITORY:-}" &&
        "${linked_repository,,}" != "${GITHUB_REPOSITORY,,}" ]]; then
        echo "State package is linked to ${linked_repository:-no repository}; expected ${GITHUB_REPOSITORY}." >&2
        return 1
    fi
}

collect_diagnostics() {
    mkdir -p "${DIAGNOSTICS_DIR}"

    if [[ -d "${PHASE_DIR}" ]]; then
        find "${PHASE_DIR}" -maxdepth 1 -type f -printf '%f\n' \
            | sort >"${DIAGNOSTICS_DIR}/completed-phases.txt"
    fi
    if [[ -f "${SAVED_DIGEST_FILE}" ]]; then
        cp "${SAVED_DIGEST_FILE}" \
            "${DIAGNOSTICS_DIR}/saved-state-digest.txt"
    fi
    if fetch_package_metadata; then
        jq '{
          name,
          visibility,
          repository: .repository.full_name,
          version_count,
          html_url
        }' "${WORK_DIR}/package.json" \
            >"${DIAGNOSTICS_DIR}/package-metadata.json"
    fi

    rad app list --output json \
        >"${DIAGNOSTICS_DIR}/rad-app-list.json" 2>&1 || true
    oras manifest fetch --descriptor "${STATE_REFERENCE}" \
        >"${DIAGNOSTICS_DIR}/state-descriptor.json" 2>&1 || true

    local cluster
    for cluster in "${CONTROL_PLANE_A}" "${CONTROL_PLANE_B}"; do
        if ! k3d cluster list --no-headers \
            | awk '{print $1}' \
            | grep -Fxq "${cluster}"; then
            continue
        fi

        kubectl --context "k3d-${cluster}" get pods -A -o wide \
            >"${DIAGNOSTICS_DIR}/${cluster}-pods.txt" 2>&1 || true
        kubectl --context "k3d-${cluster}" get events -A \
            --sort-by=.lastTimestamp \
            >"${DIAGNOSTICS_DIR}/${cluster}-events.txt" 2>&1 || true

        local component
        for component in applications-rp dynamic-rp bicep-de controller ucp; do
            kubectl --context "k3d-${cluster}" logs \
                -n radius-system \
                -l "app.kubernetes.io/name=${component}" \
                --all-containers --tail=300 \
                >"${DIAGNOSTICS_DIR}/${cluster}-${component}.log" \
                2>&1 || true
        done
    done

    if [[ -f "${HOST_WORKLOAD_KUBECONFIG}" ]]; then
        kubectl --kubeconfig "${HOST_WORKLOAD_KUBECONFIG}" get all -A -o wide \
            >"${DIAGNOSTICS_DIR}/workload-resources.txt" 2>&1 || true
        kubectl --kubeconfig "${HOST_WORKLOAD_KUBECONFIG}" get deployment \
            -n "${WORKLOAD_NAMESPACE}" -l "${SELECTOR}" -o yaml \
            >"${DIAGNOSTICS_DIR}/workload-deployment.yaml" 2>&1 || true
    fi
}

delete_state_manifest() {
    local status

    if manifest_exists; then
        status=0
    else
        status=$?
    fi
    if ((status == 1)); then
        return 0
    fi
    if ((status != 0)); then
        return "${status}"
    fi

    configure_package_api
    local versions
    versions="$(gh api --paginate \
        "${PACKAGE_API}/versions?per_page=100")"

    local -a version_ids
    mapfile -t version_ids < <(
        jq --slurp --raw-output --arg tag "${RADIUS_STATE_ARCHIVE}" \
            '[.[][] |
              select(any(.metadata.container.tags[]?; . == $tag)) |
              .id] | .[]' <<<"${versions}"
    )
    if ((${#version_ids[@]} != 1)); then
        echo "Expected one GHCR package version for ${STATE_REFERENCE}," \
            "found ${#version_ids[@]}." >&2
        return 1
    fi

    local version_count
    version_count="$(gh api "${PACKAGE_API}" --jq '.version_count')"
    if ((version_count == 1)); then
        echo "Refusing to delete the final version of precreated package ${RADIUS_STATE_REGISTRY}." >&2
        return 1
    fi
    gh api --method DELETE \
        "${PACKAGE_API}/versions/${version_ids[0]}"

    local _
    for _ in {1..30}; do
        if manifest_exists; then
            sleep 1
            continue
        else
            status=$?
        fi
        if ((status == 1)); then
            break
        fi
        return "${status}"
    done

    if manifest_exists; then
        echo "State manifest still exists after cleanup: ${STATE_REFERENCE}" >&2
        return 1
    fi
    fetch_package_metadata
    verify_package_metadata
    oras manifest fetch --descriptor \
        "${RADIUS_STATE_REGISTRY}:${BOOTSTRAP_TAG}" >/dev/null
}

cleanup() {
    local result=$?
    local cleanup_result=0
    set +e

    if ((result != 0)); then
        collect_diagnostics
    fi

    if [[ -f "${STATE_OWNED_MARKER}" ]]; then
        delete_state_manifest || cleanup_result=1
    fi
    k3d cluster delete "${CONTROL_PLANE_A}" >/dev/null 2>&1
    k3d cluster delete "${CONTROL_PLANE_B}" >/dev/null 2>&1
    k3d cluster delete "${WORKLOAD_CLUSTER}" >/dev/null 2>&1
    docker rm --force "${REGISTRY_CONTAINER}" >/dev/null 2>&1
    docker network rm "${NETWORK_NAME}" >/dev/null 2>&1
    rm -rf "${WORK_DIR}"

    if ((cleanup_result == 0)); then
        append_summary "- State version cleanup: succeeded"
    else
        append_summary "- State version cleanup: failed"
    fi
    if ((result == 0 && cleanup_result != 0)); then
        result="${cleanup_result}"
    fi
    exit "${result}"
}

cluster_exists() {
    local cluster="$1"
    k3d cluster list --no-headers \
        | awk '{print $1}' \
        | grep -Fxq "${cluster}"
}

write_registry_config() {
    cat >"${REGISTRY_CONFIG}" <<EOF
mirrors:
  "${CLUSTER_REGISTRY}":
    endpoint:
      - "http://${CLUSTER_REGISTRY}"
EOF
}

start_registry() {
    docker network create "${NETWORK_NAME}" >/dev/null
    docker run --detach --rm \
        --name "${REGISTRY_CONTAINER}" \
        --network "${NETWORK_NAME}" \
        --network-alias "${REGISTRY_ALIAS}" \
        --publish 127.0.0.1:5000:5000 \
        registry:2 >/dev/null

    local _
    for _ in {1..30}; do
        if curl -fsS http://127.0.0.1:5000/v2/ >/dev/null; then
            return 0
        fi
        sleep 1
    done
    echo "Local OCI registry did not become ready." >&2
    return 1
}

publish_branch_artifacts() {
    local image
    for image in ucpd applications-rp dynamic-rp controller bicep; do
        docker push \
            "${LOCAL_DOCKER_REGISTRY}/${image}:${DOCKER_TAG_VERSION}"
    done

    cp "${SOURCE_APP_FILE}" "${APP_FILE}"
    cat >"${WORK_DIR}/bicepconfig.json" <<EOF
{
  "experimentalFeaturesEnabled": {
    "extensibility": true
  },
  "extensions": {
    "radius": "br:biceptypes.azurecr.io/radius:latest"
  }
}
EOF
    bicep build "${APP_FILE}" --stdout >/dev/null
}

create_workload_cluster() {
    k3d cluster create "${WORKLOAD_CLUSTER}" \
        --network "${NETWORK_NAME}" \
        --registry-config "${REGISTRY_CONFIG}" \
        --k3s-arg "--disable=traefik@server:*" \
        --wait

    k3d kubeconfig get "${WORKLOAD_CLUSTER}" \
        >"${HOST_WORKLOAD_KUBECONFIG}"
    cp "${HOST_WORKLOAD_KUBECONFIG}" \
        "${INTERNAL_WORKLOAD_KUBECONFIG}"

    local cluster_key
    local workload_ip
    cluster_key="$(kubectl \
        --kubeconfig "${INTERNAL_WORKLOAD_KUBECONFIG}" \
        config view --minify \
        -o jsonpath='{.contexts[0].context.cluster}')"
    workload_ip="$(docker inspect \
        --format \
        "{{(index .NetworkSettings.Networks \"${NETWORK_NAME}\").IPAddress}}" \
        "k3d-${WORKLOAD_CLUSTER}-server-0")"

    if [[ -z "${workload_ip}" ]]; then
        echo "Could not determine the workload cluster IP." >&2
        return 1
    fi

    kubectl --kubeconfig "${INTERNAL_WORKLOAD_KUBECONFIG}" \
        config set "clusters.${cluster_key}.server" \
        "https://${workload_ip}:6443" >/dev/null
    kubectl --kubeconfig "${INTERNAL_WORKLOAD_KUBECONFIG}" \
        config unset \
        "clusters.${cluster_key}.certificate-authority-data" >/dev/null
    kubectl --kubeconfig "${INTERNAL_WORKLOAD_KUBECONFIG}" \
        config set \
        "clusters.${cluster_key}.insecure-skip-tls-verify" \
        "true" >/dev/null

    kubectl --kubeconfig "${HOST_WORKLOAD_KUBECONFIG}" \
        create namespace "${WORKLOAD_NAMESPACE}"
}

configure_workspace() {
    local cluster="$1"

    kubectl config use-context "k3d-${cluster}" >/dev/null
    rad workspace create kubernetes "${WORKSPACE_NAME}" \
        --context "k3d-${cluster}" \
        --force
    rad workspace switch "${WORKSPACE_NAME}"
    rad group create default
    rad group switch default
}

install_control_plane() {
    local cluster="$1"
    local install_needs_recovery=false

    k3d cluster create "${cluster}" \
        --network "${NETWORK_NAME}" \
        --registry-config "${REGISTRY_CONFIG}" \
        --k3s-arg "--disable=traefik@server:*" \
        --wait
    kubectl config use-context "k3d-${cluster}" >/dev/null

    kubectl create namespace radius-system
    kubectl create secret generic target-kubeconfig \
        --namespace radius-system \
        --from-file=kubeconfig="${INTERNAL_WORKLOAD_KUBECONFIG}"

    if rad install kubernetes \
        --chart "${REPOSITORY_ROOT}/deploy/Chart" \
        --set database.enabled=true \
        --set global.targetCluster.enabled=true \
        --set rp.publicEndpointOverride=localhost \
        --set \
        "rp.image=${CLUSTER_REGISTRY}/applications-rp,rp.tag=${DOCKER_TAG_VERSION}" \
        --set \
        "dynamicrp.image=${CLUSTER_REGISTRY}/dynamic-rp,dynamicrp.tag=${DOCKER_TAG_VERSION}" \
        --set \
        "controller.image=${CLUSTER_REGISTRY}/controller,controller.tag=${DOCKER_TAG_VERSION}" \
        --set \
        "ucp.image=${CLUSTER_REGISTRY}/ucpd,ucp.tag=${DOCKER_TAG_VERSION}" \
        --set \
        "bicep.image=${CLUSTER_REGISTRY}/bicep,bicep.tag=${DOCKER_TAG_VERSION}"; then
        :
    else
        install_needs_recovery=true
        if ! kubectl get deployment/ucp statefulset/database \
            --namespace radius-system >/dev/null; then
            echo "Radius Helm installation did not complete." >&2
            return 1
        fi
        echo "Radius was installed, but its initial API readiness check failed."
    fi

    # Deployment Engine and dashboard are built in separate repositories. Their
    # chart defaults intentionally remain on the compatible edge channel.
    kubectl wait --for=condition=Ready pod/database-0 \
        --namespace radius-system \
        --timeout=300s
    if [[ "${install_needs_recovery}" == "true" ]]; then
        kubectl rollout restart deployment/ucp \
            --namespace radius-system
        kubectl rollout status deployment/ucp \
            --namespace radius-system \
            --timeout=300s
    fi
    kubectl wait --for=condition=Available deployment --all \
        --namespace radius-system \
        --timeout=300s
    configure_workspace "${cluster}"
}

assert_application_listed() {
    local output_file="$1"

    rad app list --output json | tee "${output_file}"
    jq --exit-status --arg app "${APP_NAME}" \
        'type == "array" and
         length == 1 and
         ((.[0].name // .[0].Name // "") == $app)' \
        "${output_file}" >/dev/null
}

assert_workload_phase() {
    local phase="$1"
    local output_file="$2"
    local pod_output="${output_file%.json}-pods.json"

    kubectl --kubeconfig "${HOST_WORKLOAD_KUBECONFIG}" \
        rollout status deployment \
        --namespace "${WORKLOAD_NAMESPACE}" \
        --selector "${SELECTOR}" \
        --timeout=300s
    kubectl --kubeconfig "${HOST_WORKLOAD_KUBECONFIG}" \
        get deployment \
        --namespace "${WORKLOAD_NAMESPACE}" \
        --selector "${SELECTOR}" \
        --output json \
        | tee "${output_file}"
    kubectl --kubeconfig "${HOST_WORKLOAD_KUBECONFIG}" \
        wait --for=condition=Ready pod \
        --namespace "${WORKLOAD_NAMESPACE}" \
        --selector "${SELECTOR}" \
        --timeout=300s
    kubectl --kubeconfig "${HOST_WORKLOAD_KUBECONFIG}" \
        get pod \
        --namespace "${WORKLOAD_NAMESPACE}" \
        --selector "${SELECTOR}" \
        --output json \
        | tee "${pod_output}"
    jq --exit-status --arg phase "${phase}" \
        '(.items | length) == 1 and
         ([.items[].spec.template.spec.containers[].args[]]
          | any(contains($phase)))' \
        "${output_file}" >/dev/null
    jq --exit-status --arg phase "${phase}" \
        '(.items | length) >= 1 and
         ([.items[].spec.containers[].args[]]
          | any(contains($phase)))' \
        "${pod_output}" >/dev/null

    local pod
    pod="$(jq --raw-output '.items[0].metadata.name' "${pod_output}")"
    local _
    for _ in {1..30}; do
        if kubectl --kubeconfig "${HOST_WORKLOAD_KUBECONFIG}" \
            logs --namespace "${WORKLOAD_NAMESPACE}" "${pod}" \
            | grep -Fq "${phase}"; then
            return 0
        fi
        sleep 1
    done
    echo "The running workload did not log phase ${phase}." >&2
    return 1
}

assert_absent_from_control_plane() {
    local cluster="$1"
    local resources

    resources="$(kubectl --context "k3d-${cluster}" get all \
        --all-namespaces \
        --selector "${SELECTOR}" \
        --output name)"
    if [[ -n "${resources}" ]]; then
        echo "Workload resources unexpectedly exist on ${cluster}:" >&2
        echo "${resources}" >&2
        return 1
    fi
}

deploy_phase() {
    local phase="$1"
    local environment_id

    environment_id="$(rad env show "${ENVIRONMENT_NAME}" \
        --output json \
        | jq --slurp --raw-output \
            'map(select(type == "object")) | first | (.id // .Id // empty)')"
    if [[ -z "${environment_id}" || "${environment_id}" == "null" ]]; then
        echo "Could not resolve the Radius environment ID." >&2
        return 1
    fi

    rad deploy "${APP_FILE}" \
        --environment "${ENVIRONMENT_NAME}" \
        --parameters "environment=${environment_id}" \
        --parameters "deploymentPhase=${phase}"
}

manifest_exists() {
    local output

    if output="$(oras manifest fetch --descriptor \
        "${STATE_REFERENCE}" 2>&1)"; then
        return 0
    fi
    if grep -Eqi '(^|[^0-9])(404|not found)([^0-9]|$)' <<<"${output}"; then
        return 1
    fi

    echo "${output}" >&2
    return 2
}

assert_state_absent() {
    local status

    if manifest_exists; then
        echo "Run-specific state already exists: ${STATE_REFERENCE}" >&2
        return 1
    else
        status=$?
    fi
    if ((status != 1)); then
        return "${status}"
    fi
}

state_digest() {
    oras manifest fetch --descriptor "${STATE_REFERENCE}" \
        | jq --exit-status --raw-output '.digest'
}

phase_validate_state_package() {
    begin_phase "validate-state-package" "Validate private state package"
    mkdir -p "${WORK_DIR}" "${DIAGNOSTICS_DIR}"
    fetch_package_metadata
    verify_package_metadata
    oras manifest fetch --descriptor \
        "${RADIUS_STATE_REGISTRY}:${BOOTSTRAP_TAG}" \
        >"${WORK_DIR}/bootstrap-descriptor.json"
    assert_state_absent
    mark_phase "validate-state-package"
    append_summary "- Package: \`${RADIUS_STATE_REGISTRY}\`"
    append_summary "- Visibility: $(jq -r '.visibility' \
        "${WORK_DIR}/package.json")"
}

phase_prepare_workload() {
    begin_phase "prepare-workload" "Prepare target workload cluster"
    require_phase "validate-state-package"
    write_registry_config
    start_registry
    publish_branch_artifacts
    create_workload_cluster
    mark_phase "prepare-workload"
}

phase_install_initial_control_plane() {
    begin_phase \
        "install-initial-control-plane" \
        "Install first Radius control plane"
    require_phase "prepare-workload"
    install_control_plane "${CONTROL_PLANE_A}"
    rad startup
    mark_phase "install-initial-control-plane"
}

phase_deploy_initial() {
    begin_phase "deploy-initial" "Deploy and verify initial application"
    require_phase "install-initial-control-plane"
    rad env create "${ENVIRONMENT_NAME}" \
        --kubernetes-namespace "${WORKLOAD_NAMESPACE}"
    deploy_phase "before-restore"
    assert_application_listed \
        "${DIAGNOSTICS_DIR}/apps-before-restore.json"
    assert_workload_phase "before-restore" \
        "${DIAGNOSTICS_DIR}/workload-before-restore.json"
    assert_absent_from_control_plane "${CONTROL_PLANE_A}"
    mark_phase "deploy-initial"
}

phase_persist_state() {
    begin_phase "persist-state" "Persist Radius state to GHCR"
    require_phase "deploy-initial"
    # The preflight proved this run-unique tag was absent. Claim it before
    # shutdown so cleanup removes any tag left by a partially failed upload.
    touch "${STATE_OWNED_MARKER}"
    rad shutdown
    local saved_digest
    saved_digest="$(state_digest)"
    printf '%s\n' "${saved_digest}" >"${SAVED_DIGEST_FILE}"
    mark_phase "persist-state"
    echo "Saved Radius state as ${saved_digest}."
    append_summary "- Saved state digest: \`${saved_digest}\`"
}

phase_replace_control_plane() {
    begin_phase "replace-control-plane" "Replace Radius control plane"
    require_phase "persist-state"
    k3d cluster delete "${CONTROL_PLANE_A}"
    if cluster_exists "${CONTROL_PLANE_A}"; then
        echo "The first control plane was not deleted." >&2
        return 1
    fi

    install_control_plane "${CONTROL_PLANE_B}"
    mark_phase "replace-control-plane"
}

phase_restore_state() {
    begin_phase "restore-state" "Restore and verify Radius state"
    require_phase "replace-control-plane"
    [[ -s "${SAVED_DIGEST_FILE}" ]] \
        || {
            echo "Saved state digest is missing." >&2
            return 1
        }
    rad startup
    assert_application_listed \
        "${DIAGNOSTICS_DIR}/apps-after-restore.json"

    local restored_digest saved_digest
    saved_digest="$(<"${SAVED_DIGEST_FILE}")"
    restored_digest="$(state_digest)"
    if [[ "${restored_digest}" != "${saved_digest}" ]]; then
        echo "State digest changed during restore." >&2
        return 1
    fi
    mark_phase "restore-state"
    append_summary "- Restored state digest: \`${restored_digest}\`"
}

phase_update_workload() {
    begin_phase "update-workload" "Update and verify existing workload"
    require_phase "restore-state"
    deploy_phase "after-restore"
    assert_workload_phase "after-restore" \
        "${DIAGNOSTICS_DIR}/workload-after-restore.json"
    assert_absent_from_control_plane "${CONTROL_PLANE_B}"
    mark_phase "update-workload"
    append_summary "- Result: state rehydration and workload update succeeded"
    echo "Repo Radius GHCR state rehydration succeeded."
}

phase_diagnostics() {
    append_summary ""
    append_summary "### Failure diagnostics"
    if [[ -f "${WORK_DIR}/current-phase" ]]; then
        append_summary "- Failed phase: \`$(<"${WORK_DIR}/current-phase")\`"
    fi
    append_summary "- Artifact: \`repo-radius-state-e2e-diagnostics\`"
    collect_diagnostics
}

run_all() {
    trap cleanup EXIT
    phase_validate_state_package
    phase_prepare_workload
    phase_install_initial_control_plane
    phase_deploy_initial
    phase_persist_state
    phase_replace_control_plane
    phase_restore_state
    phase_update_workload
    trap - EXIT
    cleanup
}

main() {
    local phase="${1:-all}"
    case "${phase}" in
        validate-state-package) phase_validate_state_package ;;
        prepare-workload) phase_prepare_workload ;;
        install-initial-control-plane)
            phase_install_initial_control_plane
            ;;
        deploy-initial) phase_deploy_initial ;;
        persist-state) phase_persist_state ;;
        replace-control-plane) phase_replace_control_plane ;;
        restore-state) phase_restore_state ;;
        update-workload) phase_update_workload ;;
        diagnostics) phase_diagnostics ;;
        cleanup) cleanup ;;
        all) run_all ;;
        -h | --help) usage ;;
        *)
            echo "Unknown phase: ${phase}" >&2
            usage >&2
            return 2
            ;;
    esac
}

main "$@"
